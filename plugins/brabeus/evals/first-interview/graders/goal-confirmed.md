---
type: regex
target: {source: file, path: record-log.txt}
flags: m
---
^review telos/goal [^\n]+: confirmed$
