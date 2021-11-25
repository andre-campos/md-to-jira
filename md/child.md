---
issuetype: Task
project: VTC
key: child-1
summary: Child 1
dependencies:
    - type: Epic
      epicLinkField: customfield_10101
      ticket: epic
    - type: Blocks
      ticket: child-2
    - type: Relates
      ticket: child-3
timeTracking:
    originalEstimate: 10d
    remainingEstimate: 10d
labels:
    - test
---
# Markdown
Hello from the child 1
