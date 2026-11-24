# Load your own documents and search them by meaning

`openarch load` puts documents into Hyland OpenArch with their text, cut into passages. Each passage
becomes a vector, made by a local model on Docker Model Runner. Then `openarch query --semantic`
finds the passages nearest in meaning to a question, and each user gets only the passages of the
documents they may read.

This example has three short synthetic documents, one load plan each, in [plans/](plans/):

| Plan | Document | Folder | Who may read it |
| --- | --- | --- | --- |
| `hr-pay-scale-2027.json` | Pay scale 2027 | `/Demo/HR` | alice (group demo-hr) |
| `finance-contractor-rate-card-2027.json` | Contractor rate card 2027 | `/Demo/Finance` | bob (group demo-finance) |
| `public-office-hours.json` | Office hours | `/Demo/Public` | every user, carol too |

The load sets no access entries. A document gets its access from its folder, so the plans put each
document in a folder of the permission demo, which `openarch demo load` creates with its access
entries.

## Requirements

- Docker Desktop with Docker Model Runner enabled, with host TCP access (`docker desktop enable model-runner --tcp 12434`)
- `openarch` on your `PATH`, or `OPENARCH=/path/to/openarch`
- The embedding model: `docker model pull ai/mxbai-embed-large`

## Run

```
./run.sh
PORT=<free port> ./run.sh
```

`run.sh` makes a new demo workspace in a temp folder, loads the plans, and asks three questions as
alice, bob and carol. It prints the first passage that each user gets, then `PASS` or `FAIL` from
its own check, and always deletes the stack at the end. It passes only when alice gets the pay
scale, bob the rate card, and carol neither of them.

## Step by step

```
openarch init --dir oa-load --context-name load --non-interactive
openarch demo users --dir oa-load
openarch up --dir oa-load
openarch demo load --dir oa-load
openarch load plans/ --dir oa-load
openarch query --semantic "How much is level 3 paid?" --context load-alice
openarch query --semantic "How much is level 3 paid?" --context load-bob
```

Add the users before `up`: Hyland OpenArch reads its users only at startup. `demo users` saves a
context for each user: `load-alice`, `load-bob` and `load-carol`. Run `openarch load` again and it
changes nothing: documents and vectors that are stored already are left as they are.

The vectors become searchable about a second after the load ends. `run.sh` waits for them.

## A load plan

```json
{"documents": [{"path": "/Demo/HR/pay-scale-2027",
                "metadata": {"sys_title": "Pay scale 2027"},
                "passages": [{"id": "example-hr-pay-scale-2027:1:1",
                              "location": {"text": {"page": 1, "paragraph": 1}},
                              "text": "Pay scale 2027. ..."}]}]}
```

- `path`: where the document goes. Missing folders are created, with no access entries of their own.
- `metadata`: the fields of the document. Without `sys_title`, the title is the last part of the path.
- `passages`: the text, cut by you. `id` must be unique in the repository, and `location` says where
  the passage is in the original document. `query --semantic --output json` returns both.
- A plan can also have an `original` file, stored as is next to the passages. See
  `openarch load --help`.

## Limits

- You write the load plans. The CLI does not read PDF or Office files, and does not cut text into
  passages.
- The access comes from the folder, not from the plan.
- Vectors have at most 1024 dimensions. The default model, `ai/mxbai-embed-large`, makes 1024.
- `openarch demo check` checks the ten documents of the demo, not the loaded ones. `run.sh` checks
  the loaded ones.
