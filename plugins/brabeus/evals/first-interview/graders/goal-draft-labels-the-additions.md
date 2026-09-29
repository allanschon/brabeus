---
type: regex
target: {source: file, path: record-tree.txt}
flags: i
---
=== telos/goal/[^\n]+\n(?:(?!=== )[\s\S])*?(interviewer|my (guess|suggestion|idea)|suggested by)
