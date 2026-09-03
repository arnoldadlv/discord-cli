# Triage Labels

The skills speak in terms of five canonical triage roles. This file maps those roles to the actual label strings used in this repo's issue tracker.

| Label in mattpocock/skills | Label in our tracker | Meaning                                  |
| -------------------------- | -------------------- | ---------------------------------------- |
| `needs-triage`             | `needs-triage`       | Maintainer needs to evaluate this issue  |
| `needs-info`               | `needs-info`         | Waiting on reporter for more information |
| `ready-for-agent`          | `ready-for-agent`    | Fully specified, ready for an AFK agent  |
| `ready-for-human`          | `ready-for-human`    | Requires human implementation            |
| `wontfix`                  | `wontfix`            | Will not be actioned                     |

When a skill mentions a role (e.g. "apply the AFK-ready triage label"), use the corresponding label string from this table.

Edit the right-hand column to match whatever vocabulary you actually use.

## Model assignment labels

When an issue reaches `ready-for-agent`, also apply exactly one `agent:*` label
so the cheapest capable model runs it.

| Label          | Model              | Use for                                                                     |
| -------------- | ------------------ | --------------------------------------------------------------------------- |
| `agent:haiku`  | `claude-haiku-4-5` | Mechanical edits: renames, config, docs, single-file fixes with a clear spec |
| `agent:sonnet` | `claude-sonnet-5`  | Default. Well-specified features touching a few files, tests included        |
| `agent:opus`   | `claude-opus-5`    | Cross-cutting design, ambiguous specs, debugging with unknown root cause     |

Triage recommends the tier with reasoning; the maintainer confirms. Default to
`agent:sonnet` when unsure. `ready-for-human` and `wontfix` issues get no
`agent:*` label.

The labels exist on GitHub. To recreate them on a fork or new remote:

```sh
gh label create needs-triage     --color e4e669 --description "Maintainer needs to evaluate this issue"
gh label create needs-info       --color d876e3 --description "Waiting on reporter for more information"
gh label create ready-for-agent  --color 0e8a16 --description "Fully specified, ready for an AFK agent"
gh label create ready-for-human  --color 1d76db --description "Requires human implementation"
gh label create wontfix          --color ffffff --description "Will not be actioned"
gh label create agent:haiku      --color c2e0c6 --description "Run with claude-haiku-4-5"
gh label create agent:sonnet     --color 5319e7 --description "Run with claude-sonnet-5"
gh label create agent:opus       --color b60205 --description "Run with claude-opus-5"
```
