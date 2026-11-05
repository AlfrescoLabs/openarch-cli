# Install OpenArch with a local agent and the Agent Skill

An AI coding agent with a shell installs OpenArch in a project. The agent gets no OpenArch facts in its
instruction: it learns the CLI from the skill that `openarch skill install` writes to
`.claude/skills/openarch/SKILL.md`. The model runs on your machine with Docker Model Runner: no cloud
model and no API key.

`run.sh` makes a new project in a temp folder, installs the skill there, and gives the agent one task:
"Set up OpenArch for local development in ./openarch-skill-<pid>." Then it checks the result itself with
`openarch smoke-test`, and always deletes the stack at the end.

## Requirements

- Docker Desktop with Docker Model Runner enabled
- [Docker Agent](https://github.com/docker/docker-agent) (`docker agent version`)
- `openarch` on your `PATH`, or `OPENARCH=/path/to/openarch`
- A local model that passes string arguments in tool calls. The default is GLM 4.7 Flash (about 16 GB):

```
docker model pull glm-4.7-flash:31B
```

The model runs next to the OpenArch stack, so plan for about 40 GB of memory. Load one model at a time.

## Run

```
./run.sh
MODEL=<another model> ./run.sh
```

The script prints the report of the agent, and then `PASS` or `FAIL` from its own check. The exit code
is 0 only when the smoke test passes.

## How it works

- `agent.yaml` gives the agent `skills: true` and one toolset: `shell`. Docker Agent finds the skill in
  `.claude/skills/` of the folder where it runs, and the agent loads it with `read_skill`.
- `run.sh` uses `--safety restricted`, and `agent.yaml` allows only `openarch ...` commands. Every other
  command is denied without a question, and so are pipes, redirections and a second command after `;`
  or `&&`.
- `run.sh` sets `OPENARCH_CONFIG_DIR` to a temp folder, so the agent never touches your saved contexts or
  the OS credential store.
- The script does not trust the report of the agent. It runs its own smoke test on the workspace,
  and fails when the agent turned off a secure default that the task did not mention.

## Which models work

The agent passes each command as a string argument of the `shell` tool. A model works when it:

- makes real tool calls, not tool calls written as text in the answer
- passes string arguments without changes, for example `{"cmd": "openarch doctor --output json"}`
- follows a skill of about 100 lines and runs one command at a time

| Model | Result |
| --- | --- |
| `glm-4.7-flash:31B` | Works |
| OLMo 3 and OLMo 3.1 | Do not work: they write tool calls as text, or the string arguments arrive broken |

To check another model, run `MODEL=<model> ./run.sh`. Only `PASS` counts.
