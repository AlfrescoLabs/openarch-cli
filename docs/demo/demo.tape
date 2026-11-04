# Demo for the README: from an empty folder to a healthy, tested stack.
# Run from the repository root after 'make build': vhs docs/demo/demo.tape
# It needs Docker. The config goes to a temp folder, so saved contexts are not touched.
Output docs/demo/demo.gif

Set Shell "bash"
Set FontSize 18
Set Width 1400
Set Height 640
Set Padding 24
Set Theme "Catppuccin Mocha"
Set TypingSpeed 40ms
Set WindowBar Colorful
Set CursorBlink false

Hide
Type `export PATH="$PWD/bin:$PATH" OPENARCH_CONFIG_DIR="$(mktemp -d)/config" OPENARCH_CREDENTIAL_STORE=file PS1='$ '` Enter
Type `cd "$(mktemp -d /tmp/oa.XXXX)" && clear` Enter
Sleep 500ms
Show

Type "openarch doctor" Enter
Wait /\$$/
Sleep 3s

Type "openarch init --dir my-openarch --non-interactive" Enter
Wait /\$$/
Sleep 3s

Type "cd my-openarch && openarch up" Enter
Sleep 2s
Hide
Wait@600s /\$$/
Show
Sleep 2s

Type "clear && openarch smoke-test" Enter
Wait@120s /\$$/
Sleep 5s

Hide
Type "openarch clean --yes" Enter
Wait@300s /\$$/
