---
type: regex
target: {source: file, path: record-tree.txt}
---
=== telos/goal/[^\n]+\n(?:(?!=== )[\s\S])*?by: \d{4}-\d{2}-\d{2}
