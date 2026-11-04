## openarch self-update

Replace this binary with the latest release

### Synopsis

Download the latest release (or --version) from GitHub and replace this binary.

Before anything changes, self-update checks the keyless Sigstore signature of
checksums.txt (made by the release workflow of AlfrescoLabs/openarch-cli for
that tag), then the SHA-256 of the archive. If a check fails, nothing changes.
The signature check needs network access to GitHub and to Sigstore.

--check only reports if a newer release exists.

A binary installed by Homebrew or Scoop is not replaced: use 'brew upgrade
openarch' or 'scoop update openarch'. A build from source is not replaced
either. --force replaces both.

```
openarch self-update [flags]
```

### Examples

```
  openarch self-update --check
  openarch self-update
  openarch self-update --version 0.2.0
```

### Options

```
      --check            only report if a newer release exists
      --force            also replace a build from source or a package manager install
  -h, --help             help for self-update
      --version string   install this release, for example 0.2.0 (default: the latest)
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

* [openarch](openarch.md)	 - Install and operate OpenArch

