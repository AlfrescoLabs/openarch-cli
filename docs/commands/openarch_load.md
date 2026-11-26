## openarch load

Load a corpus of documents and passages from load plans

### Synopsis

Load a corpus into OpenArch from load plans. A plan is a JSON file; a folder
argument stands for every .json file in it. Each plan has an optional original
file, stored as is, and passage documents:

  {"original": {"path": "/corpus/sources/a.pdf", "file": "sources/a.pdf",
                "metadata": {...}},
   "documents": [{"path": "/corpus/a/part-1", "metadata": {...},
                  "passages": [{"id": "a:1:1", "text": "...",
                                "location": {"text": {"page": 3, "paragraph": 1}}}]}]}

A passage document is a SysFile whose file is the text of its passages. Each
passage becomes a vector, made with a local model on Docker Model Runner, and
is stored with its text and location as the embeddings of the document (see
'openarch query --semantic'). A passage id must be unique in the repository: a
vector with the id of another one replaces it.

The title of a document is the last part of its path, unless its metadata has
sys_title. The load sets no access entries: a document gets its access from
its folder, so load into folders that already have the access you want.

The load can run again. A stored document that already has the metadata is
left as it is, and one with the same file gets the metadata only. Embeddings
whose fingerprint (model and passages) is stored already are not made again.
--cache keeps the embedding files, so a new workspace loads without the model
work. --done writes a marker per loaded plan; a later run skips those plans
unless --all is given.

Before any change, the longest passage is embedded once, so a model that cannot
take it fails first. --schema adds the schemas, fields and mixin types the
repository misses (a JSON file with "schemas" and "mixinTypes", as in GET
/api/repository/model); a field that exists with another type is an error.
Only an administrator may change the content model.

A plan that fails is reported and the others are still loaded; the exit code
is then 5. Progress goes to stderr.

```
openarch load <plan>... [flags]
```

### Examples

```
  openarch load plans/ --schema model.json --mixin FcSource --cache cache/ --done loaded/
  openarch load plans/session-12.json --model hf.co/gpustack/bge-m3-gguf --hash-field fc_sourceSha256 --all
```

### Options

```
      --all                     also load plans that have a marker in --done
      --batch int               passages per embedding request (default 32)
      --cache string            folder of embedding files (Parquet) by fingerprint
      --done string             folder of markers of loaded plans
      --hash-field string       metadata field with the SHA-256 of the original file: the file must match it, and a stored document with another value is not overwritten
  -h, --help                    help for load
      --insecure                do not verify the server certificate (env OPENARCH_INSECURE)
      --max-passage-bytes int   refuse longer passages (UTF-8 bytes) instead of letting the model cut them; 0: no limit (default 8000)
      --mixin strings           mixin type for every loaded document (repeatable)
      --model string            embedding model on Docker Model Runner (1024 dimensions or fewer); default from OPENARCH_EMBED_MODEL when set (default "ai/mxbai-embed-large")
      --model-url string        OpenAI-compatible API of Docker Model Runner; default from OPENARCH_MODEL_URL when set (default "http://localhost:12434/engines/v1")
      --password string         password (insecure: it stays in the shell history; prefer --password-stdin or env OPENARCH_PASSWORD)
      --password-stdin          read the password from stdin
      --root string             folder that the original file names are relative to (default ".")
      --schema string           content model fragment to add before the load (JSON file)
      --url string              OpenArch URL (env OPENARCH_URL; default: the workspace URL)
      --user string             user name (env OPENARCH_USER; default: the workspace admin)
      --workers int             plans loaded in parallel (default 6)
```

### Options inherited from parent commands

```
      --context string    connection context to use
      --dir string        workspace directory (default: current directory)
      --no-color          disable colors (env NO_COLOR)
      --non-interactive   never prompt; fail when a value is missing
  -o, --output string     output format: text or json (env OPENARCH_OUTPUT) (default "text")
  -v, --verbose           print debug logs to stderr
```

### SEE ALSO

* [openarch](openarch.md)	 - CLI for OpenArch: install and operate Hyland OpenArch

