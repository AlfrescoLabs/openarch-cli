# Install OpenArch with a local agent

An AI agent installs OpenArch and checks it, with the tools of `openarch mcp` only. The model runs on
your machine with Docker Model Runner: no cloud model and no API key.

`run.sh` gives the agent one task, "Install OpenArch on this machine and check that it works.", then
checks the result itself with `openarch smoke-test`, and always deletes the stack at the end.

## Requirements

- Docker Desktop with Docker Model Runner enabled
- [Docker Agent](https://github.com/docker/docker-agent) (`docker agent version`)
- `openarch` on your `PATH`, or `OPENARCH=/path/to/openarch`
- A local model with tool calls. The default is OLMo 3.1 32B Instruct (about 18 GB):

```
docker model pull hf.co/unsloth/Olmo-3.1-32B-Instruct-GGUF:Q4_K_M
docker model configure --context-size 32768 hf.co/unsloth/Olmo-3.1-32B-Instruct-GGUF:Q4_K_M
```

The model runs next to the OpenArch stack, so plan for about 48 GB of memory with the 32B model. A
32k context is enough for this task and keeps the memory use lower than the model default.

## Run

```
./run.sh
MODEL=local/olmo3-7b-instruct:Q4_K_M ./run.sh
./run.sh "Install OpenArch, then show me its status."
```

The script prints the tool calls of the agent, its report, and then `PASS` or `FAIL` from its own
check. The exit code is 0 only when the smoke test passes.

## How it works

- `agent.yaml` gives the agent one toolset: `openarch mcp`. It has no shell and no file access, so
  every step goes through the CLI commands. Tools that delete data are off, because the server runs
  without `--allow-destructive`.
- `run.sh` sets `OPENARCH_WORKSPACE` and `OPENARCH_CONFIG_DIR` to a temp folder. The MCP server
  inherits them, so the agent never touches your saved contexts or the OS credential store.
- The instruction asks the model to get every fact from a real tool call. Without it, OLMo 3.1 32B
  can answer with invented tool results instead of calling the tools. `openarch mcp` gives the same
  rule to every client in its server instructions.
- The script does not trust the report of the agent. It runs its own smoke test on the workspace.

## Tested models

| Model | Result |
| --- | --- |
| `hf.co/unsloth/Olmo-3.1-32B-Instruct-GGUF:Q4_K_M` | Installs with the secure defaults; smoke test passes |
| `glm-4.7-flash:31B` | Installs; smoke test passes |
