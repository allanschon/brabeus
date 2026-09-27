# Domain-driven design

Brabeus described in the terms of domain-driven design: the language it uses, the contexts that
language holds in, what changes together, what must always be true, and which facts other parts
react to. They sit beside the [specification](../personal-context-system-v1.md), the
[C4 diagrams](../personal-context-system-c4.md) and the
[plain-language description](../personal-context-system-plain.md); where they disagree with the
specification, the specification is the design and these say where the code differs.

| document                                      | answers                                                                               |
| --------------------------------------------- | ------------------------------------------------------------------------------------- |
| [Ubiquitous language](ubiquitous-language.md) | what each term means, in plain words too, and every other name it goes by             |
| [Bounded contexts](bounded-contexts.md)       | where each term holds, which contexts are core, and how they relate — the context map |
| [Aggregates](aggregates.md)                   | what changes together, behind which root                                              |
| [Invariants](invariants.md)                   | every rule that must hold, where it is enforced, and where it does not hold           |
| [Domain events](domain-events.md)             | which facts other parts react to, and why none is an explicit event                   |
