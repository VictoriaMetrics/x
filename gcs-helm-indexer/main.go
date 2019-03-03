package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"github.com/pkg/errors"
	"io"
	"io/ioutil"
	"log"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/blang/semver"
	"github.com/ghodss/yaml"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"k8s.io/helm/pkg/chartutil"
	"k8s.io/helm/pkg/proto/hapi/chart"
	"k8s.io/helm/pkg/provenance"
	"k8s.io/helm/pkg/repo"
)

const (
	indexFile = "index.yaml"
	delimiter = "-"
)

var (
	bucket    = flag.String("b", "", "Google bucket name. Mandatory")
	project   = flag.String("p", "", "Helm project name. Mandatory")
	bucketURL = flag.String("url", "", " Google bucket url. If not provided it's generated from bucket name")
	dst       = flag.String("dst", "", "Path to generated file. If not provided output prints to Stdout")
	authPath  = flag.String("authPath", "", "Path to file with credentials. If not provided ENV variable is used")
)

type params struct {
	bucket    string
	bucketURL string
	project   string
	dst       string
	authPath  string
}

func main() {
	p, err := parseParams()
	if err != nil {
		log.Fatal(err)
	}
	if err := process(p); err != nil {
		log.Fatal(err)
	}
}

func parseParams() (params, error) {
	flag.Parse()
	p := params{}
	if *bucket == "" {
		return p, errors.New("empty bucket given")
	}
	p.bucket = *bucket
	p.bucketURL = *bucketURL
	if p.bucketURL == "" {
		p.bucketURL = generateBucketURL(p.bucket)
	}
	p.project = *project
	if p.project == "" {
		return p, errors.New("empty project given")
	}
	p.dst = *dst
	p.authPath = *authPath
	return p, nil
}

func generateBucketURL(b string) string {
	return fmt.Sprintf("https://%s.storage.googleapis.com/", b)
}

func process(p params) error {
	ctx := context.Background()
	client, err := bucketClient(ctx, p.authPath)
	if err != nil {
		return err
	}
	b := client.Bucket(p.bucket)
	index, err := index(ctx, b)
	if err != nil {
		return err
	}
	fls, err := files(ctx, p.project, b)
	if err != nil {
		return err
	}
	for v, f := range fls {
		if !index.Has(p.project, v) {
			c, d, err := loadChart(ctx, b, f)
			if err != nil {
				return err
			}
			index.Add(c.Metadata, f, p.authPath, d)
		}
	}
	exist := make(repo.ChartVersions, 0)
	for i, c := range index.Entries[p.project] {
		if _, ok := fls[c.Version]; ok {
			exist = append(exist, index.Entries[p.project][i])
		}
	}
	index.Generated = time.Now()
	index.Entries[p.project] = exist
	index.SortEntries()
	if p.dst != "" {
		return index.WriteFile(p.dst, 0644)
	}
	raw, err := yaml.Marshal(index)
	if err != nil {
		return err
	}
	fmt.Println(string(raw))
	return nil
}

func bucketClient(ctx context.Context, authPath string) (*storage.Client, error) {
	var options []option.ClientOption
	if authPath != "" {
		options = append(options, option.WithCredentialsFile(authPath))
	}
	return storage.NewClient(ctx, options...)
}

func index(ctx context.Context, b *storage.BucketHandle) (*repo.IndexFile, error) {
	i := repo.NewIndexFile()
	r, err := b.Object(indexFile).NewReader(ctx)
	if err != nil {
		if err == storage.ErrObjectNotExist {
			return i, nil
		}
		return nil, err
	}
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(data, i); err != nil {
		return i, err
	}
	i.SortEntries()
	return i, nil
}

func files(ctx context.Context, project string, b *storage.BucketHandle) (map[string]string, error) {
	iter := b.Objects(ctx, &storage.Query{
		Prefix:    project,
		Delimiter: "",
		Versions:  false,
	})
	m := make(map[string]string)
	for {
		o, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		if !strings.HasSuffix(o.Name, ".tgz") {
			continue
		}
		v, err := versionFromFile(project, delimiter, o.Name)
		if err != nil {
			return nil, errors.Wrapf(err, "unable get version from file %s", o.Name, )
		}
		m[v.String()] = o.Name
	}
	return m, nil
}

func versionFromFile(project, delimiter, file string) (semver.Version, error) {
	v := strings.Replace(file, project+delimiter, "", -1)
	v = strings.Replace(v, ".tgz", "", 1)
	return semver.Parse(v)
}

func loadChart(ctx context.Context, b *storage.BucketHandle, fileName string) (*chart.Chart, string, error) {
	r, err := b.Object(fileName).NewReader(ctx)
	if err != nil {
		return nil, "", err
	}
	var buf bytes.Buffer
	tee := io.TeeReader(r, &buf)
	c, err := chartutil.LoadArchive(tee)
	if err != nil {
		return nil, "", err
	}
	d, err := provenance.Digest(&buf)
	return c, d, err
}
