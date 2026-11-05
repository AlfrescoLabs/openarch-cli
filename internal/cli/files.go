package cli

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/AlfrescoLabs/openarch-cli/internal/api"
	"github.com/AlfrescoLabs/openarch-cli/internal/output"
)

func newDocumentsUploadCmd(app *App, f *connFlags) *cobra.Command {
	var parent, name, title, description, mimeType string
	cmd := &cobra.Command{
		Use:   "upload <file>",
		Short: "Upload a file as a new document",
		Long: `Upload a local file as a new SysFile document under a parent, given by id
or path.

The name is the file name unless --name is set. The MIME type comes from the
file extension, else from the first bytes of the file, unless --mime-type is
set. OpenArch stores the SHA-256 of the content; 'documents content' checks it.

OpenArch keeps the file but does not extract its text, so words inside the
file are not found by full-text queries.`,
		Example: `  openarch documents upload report.pdf --parent /projects
  openarch documents upload notes.txt --parent / --name 2026-notes.txt --title "Notes 2026"
  openarch documents upload photo.jpg --parent /case --description "Sent by Ana on 2026-09-10"`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if parent == "" {
				return output.Usage("--parent is required", "Use an id, or a path such as /.")
			}
			file, err := os.Open(args[0])
			if err != nil {
				return output.Usage("cannot read the file: "+err.Error(), "")
			}
			defer func() { _ = file.Close() }()
			if st, err := file.Stat(); err != nil || st.IsDir() {
				return output.Usage(args[0]+" is not a file", "")
			}
			if name == "" {
				name = filepath.Base(args[0])
			}
			if title == "" {
				title = name
			}
			if mimeType == "" {
				if mimeType, err = detectMimeType(file); err != nil {
					return output.Usage("cannot read the file: "+err.Error(), "")
				}
			}
			c, err := app.client(cmd, f)
			if err != nil {
				return err
			}
			doc, err := c.UploadFile(cmd.Context(), parent, name, title, description, mimeType, file)
			if err != nil {
				return err
			}
			return app.Printer.Print(doc)
		},
	}
	fl := cmd.Flags()
	fl.StringVar(&parent, "parent", "", "parent document: id, or path starting with /")
	fl.StringVar(&name, "name", "", "sys_name (default: the file name)")
	fl.StringVar(&title, "title", "", "sys_title (default: the name)")
	fl.StringVar(&description, "description", "", "sys_description")
	fl.StringVar(&mimeType, "mime-type", "", "MIME type of the content (default: from the extension or the content)")
	return cmd
}

// detectMimeType guesses the MIME type of file from its extension, else from
// its first bytes, and rewinds it.
func detectMimeType(file *os.File) (string, error) {
	if t := mime.TypeByExtension(strings.ToLower(filepath.Ext(file.Name()))); t != "" {
		return t, nil
	}
	head := make([]byte, 512)
	n, err := file.Read(head)
	if err != nil && err != io.EOF {
		return "", err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	return http.DetectContentType(head[:n]), nil
}

func newDocumentsContentCmd(app *App, f *connFlags) *cobra.Command {
	var out string
	var force bool
	cmd := &cobra.Command{
		Use:   "content <id-or-path>",
		Short: "Download the file of a document",
		Long: `Download the file of a document to --out, and check its SHA-256 against
the digest stored by OpenArch.

--out - writes the file to stdout. Then nothing else is printed there, so it
cannot be used with --output json. An existing file is replaced only with
--force.`,
		Example: `  openarch documents content /projects/report.pdf --out report.pdf
  openarch documents content /notes.txt --out - | less
  openarch documents content dae8e1bb-ef08-44e5-81ae-451d785a3643 --out copy.pdf --output json`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if out == "" {
				return output.Usage("--out is required", "Use a file name, or - for stdout.")
			}
			if out == "-" && app.Printer.Format() == output.JSON {
				return output.Usage("--out - cannot be used with --output json", "Use --out <file> to get the JSON result.")
			}
			c, err := app.client(cmd, f)
			if err != nil {
				return err
			}
			if out == "-" {
				_, err := c.Download(cmd.Context(), args[0], app.Streams.Out)
				return err
			}
			if _, err := os.Stat(out); err == nil && !force {
				return output.Usage(out+" exists", "Use --force to replace it.")
			}
			// Write to a temp file in the same folder, so a failed download
			// leaves no partial file.
			tmp, err := os.CreateTemp(filepath.Dir(out), "."+filepath.Base(out)+".*")
			if err != nil {
				return err
			}
			defer func() { _ = os.Remove(tmp.Name()) }()
			res, err := c.Download(cmd.Context(), args[0], tmp)
			if cerr := tmp.Close(); err == nil {
				err = cerr
			}
			if err != nil {
				return err
			}
			if err := os.Rename(tmp.Name(), out); err != nil {
				return err
			}
			return app.Printer.Print(contentResult{DownloadResult: res, Path: out})
		},
	}
	fl := cmd.Flags()
	fl.StringVar(&out, "out", "", "file to write, or - for stdout")
	fl.BoolVar(&force, "force", false, "replace an existing file")
	return cmd
}

// contentResult is the result of 'documents content'.
type contentResult struct {
	api.DownloadResult
	Path string `json:"path"`
}

func (r contentResult) WriteText(w io.Writer) error {
	_, err := fmt.Fprintf(w, "Wrote %s (%d bytes, %s) to %s\nsha256: %s\n", r.Filename, r.Length, r.MimeType, r.Path, r.SHA256)
	return err
}

func newDocumentsListCmd(app *App, f *connFlags) *cobra.Command {
	var o api.ListOptions
	cmd := &cobra.Command{
		Use:   "list [folder]",
		Short: "List the documents in a folder",
		Long: `List the documents in a folder, given by id or path (default: the root "/"),
sorted by name.

With --output json, the result is the server query result as is.`,
		Example: `  openarch documents list
  openarch documents list /projects --limit 20 --offset 20
  openarch documents list /projects --output json`,
		Args: maxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			folder := "/"
			if len(args) == 1 {
				folder = args[0]
			}
			c, err := app.client(cmd, f)
			if err != nil {
				return err
			}
			res, err := c.ListChildren(cmd.Context(), folder, o)
			if err != nil {
				return err
			}
			return app.Printer.Print(listResult{res})
		},
	}
	fl := cmd.Flags()
	fl.Int64Var(&o.Limit, "limit", 100, "maximum number of documents")
	fl.Int64Var(&o.Offset, "offset", 0, "number of documents to skip")
	return cmd
}

// listResult prints the children of a folder with their file size.
type listResult struct {
	api.QueryResult
}

func (r listResult) WriteText(w io.Writer) error {
	var b strings.Builder
	if len(r.Documents) == 0 {
		b.WriteString("No documents.\n")
	}
	wName, wType := 4, 4
	for _, d := range r.Documents {
		wName = max(wName, len(d.Name))
		wType = max(wType, len(d.PrimaryType))
	}
	for _, d := range r.Documents {
		size := "-"
		if d.Blob != nil {
			size = fmt.Sprint(d.Blob.Length)
		}
		fmt.Fprintf(&b, "%-*s  %-*s  %10s  %s\n", wName, d.Name, wType, d.PrimaryType, size, d.Modified)
	}
	if len(r.Documents) > 0 {
		fmt.Fprintf(&b, "\n%d to %d of %d\n", r.Offset+1, r.Offset+int64(len(r.Documents)), r.TotalCount)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// maxArgs is cobra.MaximumNArgs with a usage error.
func maxArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) > n {
			return output.Usage(fmt.Sprintf("%q accepts at most %d argument(s), got %d", cmd.CommandPath(), n, len(args)),
				"Run '"+cmd.CommandPath()+" --help' for usage.")
		}
		return nil
	}
}
