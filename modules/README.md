# Modules

A module is a directory with a manifest, `module.yaml`. It declares a set of record kinds (each
with its required and optional fields and a freshness threshold), an interview prompt per kind, a
summary template that renders the module's part of the context block, the evidence adapters its
claims may use, and which of the kernel's two fixed profiles it runs under —
`working-memory` or `ratified-record`. The profile, not the manifest, decides who may write, and
whether a record needs to be ratified before it renders. See `docs/personal-context-system-v1.md`
§6 for the full contract.

The kernel does not yet enforce any of this. Loading a module, validating its manifest, budgeting
its share of the context block, and routing writes and searches through its kinds is the module
contract itself — that lands in M1, once profiles and modules exist as real kernel concepts. What
ships in this seed is the *shape*: the five manifests below, as documents of intent, so the layout
exists from the first commit and later work has something concrete to build against.

Five modules ship: `memory` under `working-memory` — the assistant's per-machine working notes,
and the only module with scope keys — and four core modules under `ratified-record`: `identity`,
`telos`, `health` and `finance`. See §7 for what each holds.
