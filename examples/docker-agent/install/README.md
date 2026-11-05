# Install OpenArch with a local agent

An AI agent installs OpenArch and checks it, with the tools of `openarch mcp` only. The model runs on
your machine with Docker Model Runner: no cloud model and no API key.

`run.sh` gives the agent one task, "Install OpenArch on this machine and check that it works.", then
checks the result itself with `openarch smoke-test`, and always deletes the stack at the end.

## Requirements

- Docker Desktop with Docker Model Runner enabled
- [Docker Agent](https://github.com/docker/docker-agent) (`docker agent version`)
- `openarch` on your `PATH`, or `OPENARCH=/path/to/openarch`
- A local model with tool calls. The default is GLM 4.7 Flash (about 16 GB):

```
docker model pull glm-4.7-flash:31B
```

The model runs next to the OpenArch stack, so plan for about 40 GB of memory. Load one model at a time.

## Run

```
./run.sh
MODEL=<another model> ./run.sh
./run.sh "Install OpenArch, then show me its status."
```

The script prints the tool calls of the agent, its report, and then `PASS` or `FAIL` from its own
check. The exit code is 0 only when the smoke test passes.

## How it works

- `agent.yaml` gives the agent one toolset: `openarch mcp`. It has no shell and no file access, so
  every step goes through the CLI commands. Tools that delete data are off, because the server runs
  without `--allow-destructive`, and the `init` tool cannot turn security off, because it runs without
  `--allow-insecure`.
- `run.sh` sets `OPENARCH_WORKSPACE` and `OPENARCH_CONFIG_DIR` to a temp folder. The MCP server
  inherits them, so the agent never touches your saved contexts or the OS credential store.
- The instruction asks the model to get every fact from a real tool call. Without it, some models
  answer with invented tool results instead of calling the tools. `openarch mcp` gives the same rule
  to every client in its server instructions.
- The script does not trust the report of the agent. It runs its own smoke test on the workspace,
  and fails when the agent turned off a secure default that the task did not mention.

## Which models work

A model works when it makes real tool calls, not tool calls written as text in the answer, and
when it waits for each result before the next step.

| Model | Result |
| --- | --- |
| `glm-4.7-flash:31B` | Works |
| OLMo 3 and OLMo 3.1 | Do not work: they write the tool calls as text |

To check another model, run `MODEL=<model> ./run.sh`. Only `PASS` counts.
