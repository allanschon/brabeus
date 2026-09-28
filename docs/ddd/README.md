# Domain-driven design

These documents describe Brabeus in the terms of domain-driven design: the language it uses, the
contexts in which that language holds, what changes together, what must always be true, and which
facts other parts of the system react to. They sit beside the
[specification](../personal-context-system-v1.md), the [C4 diagrams](../personal-context-system-c4.md)
and the [plain-language description](../personal-context-system-plain.md).

The specification is the design. Where these documents find the code doing something the
specification does not say, they record the difference rather than treating the code as the
authority, so that a reader can tell a deliberate rule from an accident of implementation.

| document                                      | answers                                                                               |
| --------------------------------------------- | ------------------------------------------------------------------------------------- |
| [Ubiquitous language](ubiquitous-language.md) | what each term means, in plain words too, and every other name it goes by             |
| [Bounded contexts](bounded-contexts.md)       | where each term holds, which contexts are core, and how they relate — the context map |
| [Aggregates](aggregates.md)                   | what changes together, behind which root                                              |
| [Invariants](invariants.md)                   | every rule that must hold, where it is enforced, and where it does not hold           |
| [Domain events](domain-events.md)             | which facts other parts react to, and why none is an explicit event                   |
