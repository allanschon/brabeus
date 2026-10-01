---
type: regex
target: {source: file, path: record-log.txt}
flags: m
---
^Q: the claim "All chapters of the guide are drafted" failed on \d{4}-\d{2}-\d{2} \(2 chapters still undrafted\)\. Still right\? Progress since \d{4}-\d{2}-\d{2}\?
