---
type: regex
target: {source: file, path: record-log.txt}
flags: m
---
^review telos/goal ship-the-guide: later\n(?:(?!=== commit)[^\n]*\n)*?A: later$
