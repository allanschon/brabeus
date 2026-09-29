# The interview eval suite

The `interview` skill's rules are prose to a model, so nothing in the kernel can enforce them. These
cases check them the only way they can be checked: a scripted person holds an interview with a real
kernel, and the graders read what that kernel's repository holds afterwards. Each case gets its
own throwaway kernel on a temporary record with synthetic data, torn down when the case ends.

| case                                    | what it proves                                                                                                                                                   |
| --------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `first-interview`                       | A first interview on an empty record turns the person's answers into a confirmed register, value and goal, and leaves a thread for sleep, which no module holds. |
| `check-in-opens-with-the-contradiction` | A check-in opens on a failed claim, and a confirmed goal changes only through a `corrected` review carrying the person's words.                                   |
| `a-thread-for-a-topic-no-module-holds`  | Once a module covers a thread's topic, the interview takes it up, writes the draft there, and deletes the thread.                                                |
| `stop-loses-nothing`                    | "stop" ends the interview at once. The drafts are already in the record, and nothing is confirmed or snoozed.                                                    |
| `review-carries-the-persons-words`      | A value is drafted in the person's words, and its review carries their answer verbatim.                                                                          |

## When to run it, and what it costs

Run it when the `interview` skill changes and before a release, not in CI: every case is a real
model conversation on your account. One pass of the five cases, two runs each, cost $3.20 and
took six and a half minutes on 2026-09-29, with the default model and a Sonnet judge. Pass
`--max-cost-usd` to cap a run.

## How to run it

From `plugins/brabeus`, with Docker, `git`, `curl` and `jq` on the machine:

```sh
docker build -t brabeus:eval ../..      # once, and again after the kernel changes
claude plugin eval . --scaffold --mocks off \
  --allow-tools 'mcp__plugin_brabeus_brabeus__*' \
  --ablation none --judge-model sonnet --runs 2 --threshold 0.8 \
  --no-publish --report evals/results/report.html
```

Why each flag is there:

- `--scaffold` runs each case's `scaffold.sh`, which starts the case's kernel.
- `--mocks off` starts the plugin's real MCP registration, pointed at that kernel.
- `--allow-tools` grants the kernel's tools. The broader `mcp__*` does not grant them; the eval
  denies each call as if no grant had been given.
- `--ablation none` skips the default no-plugin arm. Without the plugin there is no MCP server, so
  that arm could only score 0, at the price of a second run of every case.
- `--judge-model sonnet` is for the judged graders. The default small judge misses the difference
  between an inference that is labelled as the interviewer's and one that is not.
- `--no-publish` keeps the report local. Traces carry the machine's host name, because the
  SessionStart hook reports it.
- `--threshold 0.8` absorbs the variance of the judged graders, and so it is not the verdict. A case
  with five or more graders stays at or above 0.8 with one of them failing, whichever it is. Read
  the report: any FAIL on a grader that is not `llm` is a regression.
- Keep the default concurrency of 1. Each case's kernel has a fixed port, so two runs of the same
  case at once would collide.

`evals/results/` is ignored by git.

## How a case reaches its kernel

These facts were established by running a case, and the plugin's `.mcp.json` depends on them.

**The URL.** An eval's child process inherits an allowlisted environment that does not include
`BRABEUS_URL`. A prompt's frontmatter may set `EVAL_*` variables only. `.mcp.json` therefore reads
`${BRABEUS_URL:-}${EVAL_BRABEUS_URL:-}/mcp`: in production `EVAL_BRABEUS_URL` is unset and the URL
is unchanged, and in an eval `BRABEUS_URL` is unset and the case's URL is used. If both are ever
set, the result is either not a URL or names a host that does not exist (the kernel's host with
`http` appended), so an eval can never reach the kernel named in `BRABEUS_URL`. Two other forms were tried and fail:

- `${BRABEUS_URL:-${EVAL_BRABEUS_URL}}` fails because Claude Code does not expand a nested default,
  so the URL is invalid.
- `${BRABEUS_URL}${EVAL_BRABEUS_URL}` fails because a variable without a default that is unset
  keeps the server from starting.

In production, the one change is the error for an unset `BRABEUS_URL`: an invalid-URL error, where
it used to be a missing-variable error. The SessionStart hook still says the variable is unset.

**The caller's identity.** The kernel refuses a caller it cannot name. `.mcp.json` sends
`Authorization: Bearer ${BRABEUS_TOKEN:-}${EVAL_BRABEUS_TOKEN:-}`. Each case's kernel runs identity
mode `token` with the synthetic token `eval`, and each prompt sets `EVAL_BRABEUS_TOKEN: "eval"`. In
production the header carries `BRABEUS_TOKEN` for a token-mode kernel, as the hooks already do.
Without a token it is an empty `Bearer `, which tailscale identity and auth mode `none` both
ignore.

**The tool names.** A plugin's server is registered as `plugin:brabeus:brabeus`, so its tools are
`mcp__plugin_brabeus_brabeus__<tool>`. Every `tool_used` grader uses that spelling.

**What the plugin's hooks see.** The SessionStart hook reads only `BRABEUS_URL`, so every eval
session starts with the note "kernel unreachable" and the routing guard off. The interview does not
depend on either: it calls `context` itself.

## How a case is graded

`_kernel.sh`, which every scaffold sources, installs a post-receive hook in the case's record. The
hook runs after every push the kernel makes and writes two files into the workspace:

- `record-tree.txt` holds every file on `main`, each under a `=== <path>` line.
- `record-log.txt` holds every commit message, each under a `=== commit` line.

Most graders are regexes over these files, so they check what the kernel accepted, not what the
model tried. A review's question, verdict and answer are in its commit message and nowhere else:
`review identity/value family: confirmed`, then `Q:`, `Verdict:` and `A:` lines. `tool_used`
graders count attempts, including refused ones, so they are used only for something that must
never be attempted. Judged (`llm`) graders cover what no file records, such as whether an inference
was labelled, and are the only graders that may vary between runs.

A scaffold that seeds records asserts its starting state before the model starts, and fails the
case if the fixture is wrong. That way a broken fixture costs nothing, where a paid run scoring 0
would not.

## Adding a case

Copy a case directory and give its `scaffold.sh` an unused port from 18091 upward, and name the same
port in the prompt's `EVAL_BRABEUS_URL`. To seed records, use the scaffold's `call <tool> <json>`,
and assert the result with `agenda_top` or `record-tree.txt`. Give each graded fact its own grader,
so a failure names the fact that broke.

Keep the opening paragraph of the prompt. It tells the model not to wait for a person who will not
type, which a model otherwise sometimes does, ending the run after its first question. It also
tells the model to write each question and answer out. Without that, the model can hold the whole
exchange in its thinking, and a judged grader then has no conversation to read.
