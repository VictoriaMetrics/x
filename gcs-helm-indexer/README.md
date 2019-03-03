# gcs-helm-indexer - tools for creation HELM index file and Google Cloud Storage

This is really an experimental tool which solves particular one task and doesn't support many cases (file generation, rich folder structure, etc...). Feel free to touch we us and request the features.  


### Install 

```
$ go get github.com/VictoriaMetrics/x/gcs-helm-indexer
```

### Usage

```
$ gcs-helm-indexer -h
```

or 

| Flag  | Description   |  
|---|---|
|  -b | Google bucket name. Mandatory |
|  -p | Helm project name. Mandatory |
|  -url | Google bucket url. If not provided it's generated from bucket name |
| -authPath  | Path to file with credentials. If not provided ENV variable is used  |
|  -dst | Path to generated file. If not provided output prints to Stdout |  
