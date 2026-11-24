package cli

import (
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/AlfrescoLabs/openarch-cli/internal/api"
	"github.com/AlfrescoLabs/openarch-cli/internal/loader"
	"github.com/AlfrescoLabs/openarch-cli/internal/output"
)

func newLoadCmd(app *App) *cobra.Command {
	var f connFlags
	var m modelFlags
	var o loader.Options
	var schema string
	cmd := &cobra.Command{
		Use:   "load <plan>...",
		Short: "Load a corpus of documents and passages from load plans",
		Long: `Load a corpus into OpenArch from load plans. A plan is a JSON file; a folder
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
is then 5. Progress goes to stderr.`,
		Example: `  openarch load plans/ --schema model.json --mixin FcSource --cache cache/ --done loaded/
  openarch load plans/session-12.json --model hf.co/gpustack/bge-m3-gguf --hash-field fc_sourceSha256 --all`,
		Args: minArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			files, err := planFiles(args)
			if err != nil {
				return err
			}
			if schema != "" {
				data, err := os.ReadFile(schema)
				if err != nil {
					return output.Usage(err.Error(), "")
				}
				o.Schema = data
			}
			c, err := app.client(cmd, &f)
			if err != nil {
				return err
			}
			c.SetTimeout(10 * time.Minute)
			o.Model, o.EmbeddingType, o.Embed = m.model, api.EmbeddingType(m.model), loader.Embedder(m.embedder())
			o.Progress = app.Streams.Err
			res, err := loader.Load(cmd.Context(), c, files, o)
			if err != nil {
				return err
			}
			if err := app.Printer.Print(res); err != nil {
				return err
			}
			if len(res.Errors) > 0 {
				return output.Exit(output.ExitRemote, "some plans failed")
			}
			return nil
		},
	}
	f.register(cmd)
	m.register(cmd)
	fl := cmd.Flags()
	fl.StringVar(&schema, "schema", "", "content model fragment to add before the load (JSON file)")
	fl.StringSliceVar(&o.Mixins, "mixin", nil, "mixin type for every loaded document (repeatable)")
	fl.StringVar(&o.HashField, "hash-field", "", "metadata field with the SHA-256 of the original file: the file must match it, and a stored document with another value is not overwritten")
	fl.StringVar(&o.Root, "root", ".", "folder that the original file names are relative to")
	fl.StringVar(&o.Cache, "cache", "", "folder of embedding files (Parquet) by fingerprint")
	fl.StringVar(&o.Done, "done", "", "folder of markers of loaded plans")
	fl.BoolVar(&o.All, "all", false, "also load plans that have a marker in --done")
	fl.IntVar(&o.Workers, "workers", 6, "plans loaded in parallel")
	fl.IntVar(&o.Batch, "batch", 32, "passages per embedding request")
	fl.IntVar(&o.MaxPassageBytes, "max-passage-bytes", 8000, "refuse longer passages (UTF-8 bytes) instead of letting the model cut them; 0: no limit")
	return cmd
}

// planFiles expands folders to the .json files in them, in name order.
func planFiles(args []string) ([]string, error) {
	var files []string
	for _, a := range args {
		info, err := os.Stat(a)
		if err != nil {
			return nil, output.Usage(err.Error(), "")
		}
		if !info.IsDir() {
			files = append(files, a)
			continue
		}
		found, err := filepath.Glob(filepath.Join(a, "*.json"))
		if err != nil {
			return nil, err
		}
		sort.Strings(found)
		files = append(files, found...)
	}
	if len(files) == 0 {
		return nil, output.Usage("no plan files found", "Give plan files, or folders with .json files.")
	}
	return files, nil
}
