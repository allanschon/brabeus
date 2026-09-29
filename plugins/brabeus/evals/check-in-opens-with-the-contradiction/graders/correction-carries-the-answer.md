---
type: regex
target: {source: file, path: record-log.txt}
flags: m
---
^A: (?=[^\n]*February)(?:Still right, but December is optimistic; make it February\.)? ?(?:End of February is fine\.)?$
