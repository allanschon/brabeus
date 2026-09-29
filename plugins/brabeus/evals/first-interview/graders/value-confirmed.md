---
type: regex
target: {source: file, path: record-log.txt}
flags: m
---
^review identity/value [^\n]+: confirmed$
