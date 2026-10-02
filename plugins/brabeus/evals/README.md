# The interview eval suite

The `interview` skill's rules are prose to a model, so nothing in the kernel can enforce them. These
cases check them the only way they can be checked: a scripted person holds an interview with a real
kernel, and the graders read what that kernel's repository holds afterwards. Each case gets its
own throwaway kernel on a temporary record with synthetic data, torn down when the case ends.

Every grader in a case whose name does not start with `judged-` is deterministic: it reads the
record, or counts a tool call. Each judged (`llm`) grader has a case of its own, which replays the
same conversation. The suite runs at the runner's default threshold of 1.0, so any single failing
grader fails its case and the run exits 1.

| case                                             | what it proves                                                                                                                                                                                                                                                                |
| ------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `first-interview`                                | A first interview on an empty record turns the person's answers into a confirmed register, value and goal, and leaves a thread for sleep, which no module holds. The goal's draft labels what the interviewer added, and every review's answer is the person's words exactly. |
| `check-in-opens-with-the-contradiction`          | A confirmed goal with a failed claim changes only through a `corrected` review whose answer is the person's words and nothing else, and whose question starts with the kernel's agenda question, word for word, with no offer of the interview in it.                         |
| `a-thread-for-a-topic-no-module-holds`           | Once a module covers a thread's topic, the interview takes it up, writes the draft there, and deletes the thread.                                                                                                                                                             |
| `stop-loses-nothing`                             | "stop" ends the interview. The drafts are already in the record, and nothing is confirmed or snoozed.                                                                                                                                                                         |
| `later-puts-off-one-item`                        | A bare "later" in reply to a check-in item records a `later` review carrying "later" as the answer, and the interview carries on to another question instead of ending.                                                                                                       |
| `review-carries-the-persons-words`               | A value is drafted in the person's words, and its review carries their answer verbatim.                                                                                                                                                                                       |
| `judged-approval-names-the-guesses`              | Judged. When the person approves their goal, the question tells them which parts of it are the interviewer's.                                                                                                                                                                 |
| `judged-reflection-says-only-what-was-confirmed` | Judged. The closing reflection gives nothing of the interviewer's as the person's own.                                                                                                                                                                                        |
| `judged-check-in-raises-the-failure-first`       | Judged. A check-in raises the failed claim before anything else.                                                                                                                                                                                                              |
| `judged-stop-asks-nothing-more`                  | Judged. Nothing is asked or put up for approval after "stop".                                                                                                                                                                                                                 |
| `judged-session-raises-the-first-line`           | Judged. An ordinary session, not an interview, raises the failed claim on the first line once, and not again after the person says "later".                                                                                                                                   |
| `goal-claims-carry-their-size`                   | A goal written from the person's answers carries the claim's size they gave: the goal's deadline, a date taken from "the end of next year" so the case does not go stale, and an `effort` of fourteen days, recorded as the person said them.                                                                                                          |
| `judged-open-work-is-not-a-failure`              | Judged. A manual count of 2 of 10 with its deadline sixty days off is described as open work or on pace, never as failed, behind or a gap.                                                                                                                                    |
| `judged-instructions-reach-a-subagent`           | Judged. A subagent started in a session receives the person's confirmed preference, through the plugin's `SubagentStart` hook. |

## When to run it, and what it costs

Run it when the `interview` skill changes and before a release, not in CI: every case is a real
model conversation on your account. One run of each of the fourteen cases cost $3.95 and
took about eight minutes on 2026-10-02, with the default model and a Sonnet judge. `--runs 2`
doubles both and shows whether a judged verdict holds. Pass `--max-cost-usd` to cap a run.

## How to run it

From `plugins/brabeus`, with Docker, `git`, `curl` and `jq` on the machine:

```sh
docker build -t brabeus:eval ../..      # once, and again after the kernel changes
claude plugin eval . --scaffold --mocks off \
  --allow-tools 'mcp__plugin_brabeus_brabeus__*' \
  --ablation none --judge-model sonnet --runs 1 \
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
- No `--threshold`: the default, 1.0, fails the run on any failing grader. A failing `judged-`
  case can be judge variance; read its verdict in the report before changing the skill. A failing
  deterministic case is a regression.
- The first run in a clone asks you to confirm that you trust the plugin directory. `--trust-plugin`
  answers yes without asking, for a script.
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
`http` appended), so an eval can never reach the kernel named in `BRABEUS_URL`. Two other forms
were tried and fail:

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

**Never export `EVAL_BRABEUS_URL` or `EVAL_BRABEUS_TOKEN` in an ordinary shell.** Both are read
next to their production names, so with either set, every MCP tool call fails, while the
SessionStart hook, which prefers `BRABEUS_URL` and falls back to the eval names only when it is
unset, still reports the kernel reachable. If `/health`
says the kernel answers but the tools fail to connect, check for these two variables first.

**The tool names.** A plugin's server is registered as `plugin:brabeus:brabeus`, so its tools are
`mcp__plugin_brabeus_brabeus__<tool>`. Every `tool_used` grader uses that spelling.

**What the plugin's hooks see.** The SessionStart hook falls back to `EVAL_BRABEUS_URL` and
`EVAL_BRABEUS_TOKEN` when the production names are unset, so every eval session starts with the
case's block and routing text, as a real session does, and
`judged-session-raises-the-first-line` depends on that. The fallback is applied after the outbox
drain, so an eval never drains the machine's real outbox into a throwaway kernel.

## How a case is graded

`_kernel.sh`, which every scaffold sources, installs a post-receive hook in the case's record. The
hook runs after every push the kernel makes and writes two files into the workspace:

- `record-tree.txt` holds every file on `main`, each under a `=== <path>` line.
- `record-log.txt` holds every commit message, each under a `=== commit` line.

Most graders are regexes over these files, so they check what the kernel accepted, not what the
model tried. A review's question, verdict and answer are in its commit message and nowhere else:
`review identity/value family: confirmed`, then `Q:`, `Verdict:` and `A:` lines. `tool_used`
graders count attempts, including refused ones, so they are used only for something that must
never be attempted. Judged (`llm`) graders cover what no file records, such as what the closing
reflection said, and are the only graders that may vary between runs. Each judges something it
can see whole: a file, or the last message. The runner shortens a long trace to its first and last
turns, so a judge given `focus: trace` sees only the start and the end of the conversation.

A scaffold that seeds records asserts its starting state before the model starts, and fails the
case if the fixture is wrong. That way a broken fixture costs nothing, where a paid run scoring 0
would not.

## Adding a case

Copy a case directory and give its `scaffold.sh` an unused port from 18091 upward, and name the same
port in the prompt's `EVAL_BRABEUS_URL`. To seed records, use the scaffold's `call <tool> <json>`,
and assert the result with `agenda_top` or `record-tree.txt`. Give each graded fact its own grader,
so a failure names the fact that broke, and give each judged grader a case of its own, named
`judged-…`. A case mixing the two would fail on judge variance, or need a lower threshold that lets a
deterministic failure through.

Keep the opening paragraph of the prompt. It tells the model not to wait for a person who will not
type, which a model otherwise sometimes does, ending the run after its first question. It also
tells the model to write each question and answer out. Without that, the model can hold the whole
exchange in its thinking, and a judged grader then has no conversation to read.
