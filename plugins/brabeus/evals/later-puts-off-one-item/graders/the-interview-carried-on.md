---
type: regex
target: {source: file, path: record-log.txt}
flags: m
---
^(?:identity|telos|memory)/[a-z]+: (?!dry$|craft$|ship-the-guide$)[a-z0-9-]+$
