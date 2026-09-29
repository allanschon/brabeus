---
type: regex
target: {source: file, path: record-tree.txt}
flags: i
---
=== health/[^\n]+\n(?:(?!=== )[\s\S])*?(five|5) hours
