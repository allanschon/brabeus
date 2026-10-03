# Modules

A module is a directory with a manifest, `module.json`. The manifest declares a set of record
kinds, each with its required and optional fields and, usually, a freshness threshold and an
interview prompt.
It also declares a summary template that renders the module's part of the context block, the
evidence adapters its claims may use, and which of the kernel's two fixed profiles it runs under,
`working-memory` or `ratified-record`. The profile, not the manifest, decides who may write and
whether a record must be confirmed by the person before it renders as theirs. Keeping those rules
out of the manifest means no module can loosen them. Section 6 of the specification is the full
contract.

Two things a manifest cannot say, and the kernel refuses to start if one tries:

- A `working-memory` module has no budget and no summary template, because its records are
  searched and never rendered into the context block.
- A core module's audience is `self`, and a manifest that declares otherwise is refused, so that
  no configuration can make the person's own record readable by another program.

A ratified kind's `freshness_days` and `interview` are optional. A kind without `freshness_days`
is never due because of its age; its records still come up as drafts, through a failed claim, or
through a `due_field`. A kind without `interview` is asked "Is this still right?" when a record
goes stale, so a module never produces an empty question. From M2 a kind may also name a
`due_field`, a date field after which a record is due if it has not been reviewed since; `telos`
uses it for a decision's `revisit` date, because a decision is better asked on the date the person
chose than after a fixed age.

A kind may carry two more optional keys. `first` is the question the interview asks when nothing of
that kind is on file yet. `timeless` exempts a kind from the working-memory freshness lint, which
lists any record that is neither timeless, dated nor a pointer. A
ratified-record module lists in `onboarding` the kinds to ask for, in order, when none of that kind
is on file, and each kind it names must carry a `first` question.

A working-memory module may carry two more. `layout` says whether the module's paths follow the
store's own tree (`free`) or `<module>/<kind>/<slug>.md` (`kind`). `legacy_types` is the map the
one-time migration uses to give records written before modules a kind.

The specification adds `lenses` and `draft` per kind and `intro` per module, for the
conversational interview. The kernel validates their shape and attaches no meaning to them.

The kernel loads and validates the manifests when it starts and refuses a set it does not
understand, because an ignored key is a declaration the author believes is in force and is not: a
misspelling, or a key from a newer kernel, would otherwise change behaviour silently. It enforces each module's kinds on every write and renders each ratified-record
module's summary into the context block.

Five modules ship. `memory` runs under `working-memory`: it holds the assistant's working notes,
and it declares `scope_keys`, so the plugin fills in the machine and project when the assistant
writes. Every record carries a scope, `global` by default; in this version only a working-memory
module may declare `scope_keys`. The four core modules, `identity`, `telos`, `health` and
`finance`, run under `ratified-record`. Section 7 of the specification describes what each holds.

Each of the four ratified-record modules ships `summary.md.tmpl`, a `text/template` over
`internal/block.Data`. The kernel adds three functions, `first`, `date` and `age`, and
deliberately no others; `text/template`'s own built-in functions, such as `index`, `or` and
`printf`, are available as usual. The set is closed because a template ships with a module and must
only format the records it is given: a function that could read the environment or a file would
let a module template print a deployment's secrets into the context block. A new function is a
kernel change, reviewed like one.

Every record a template ranges over is a `block.Rec`, the stored record plus `.Mark`:
`" (unconfirmed)"` until the record is reviewed, and `""` after (spec §10). A shipped template
writes `{{.Mark}}` at the end of every line that names a record, so a draft can never render as
the person's confirmed word.

## Writing a view

The kernel serves a read-only view of the record at `/view/`: a home page carrying the context
block, and one page per module listing that module's records. A ratified-record module says how
each kind looks with an optional `view` key on the kind. A `working-memory` module may not declare
one, because its records are notes, listed by description (or by name when the description is empty) and never laid out.

`view` takes one of two forms, and declaring both is refused.

```json
"view": {"layout": "cards", "title": "dimension", "fields": ["text"], "sort": "dimension"}
"view": {"template": "view/goal.html.tmpl"}
```

The first names one of the kernel's layouts, `cards`, `table` or `list`. `title` is the field that
heads each record, `fields` are the others to show, and `sort` names the field the records are
sorted by. Every field a view names must be one the kind declares. The second names a template file inside the module,
ending `.html.tmpl`, which renders the whole kind itself.

A kind with no `view` is a `list` titled by its first declared field, showing every declared field,
so a module that says nothing about the view still has a page.

### What a template receives

A view template is an `html/template`. It is executed once per kind with a `KindPage`:

| field | type | meaning |
|---|---|---|
| `.Module`, `.Kind` | string | the module and kind names |
| `.Title` | string | the kind as a heading, such as `Goal` |
| `.View` | `module.View` | the kind's declared view |
| `.Now` | `time.Time` | when the page is rendered |
| `.Records` | `[]Record` | the kind's active records, least recently confirmed first |

The kernel prints the section heading itself, so a template renders the records only. Each
`Record` carries `Path`, `Module`, `Kind`, `Name`, `Description`, `ID`, `Scope` and `Revision`
(strings); `Fields`, a map from each declared field to its text; `Body`, the record's markdown
already rendered to safe HTML; `Snoozes` and `Snoozed`; `Claims`, a goal's claims with their
measured state; `Serves`, a list of `Name` and `Href`; and the values the kernel works out because
a template cannot: `ScopeKind` (`global`, `machine` or `project`), `Freshness` (`Reviewed`, `Age`,
`Every`, `Unconfirmed`, `Overdue`, and `Class`, which is `draft`, `unknown`, `due` or empty),
`DaysLeft` (whole days to a `by` date, nil when there is none or it has passed), `DaysPast`, and
`DaysText`, the same days as the page says them: `2 days left`, `1 day left`, `due today`, `1 day
past`, or empty. `Anchor` is the record's id on its page, `r-<kind>-<name>`; give the element
that holds a record `id="{{.Anchor}}"`, because every link to the record the kernel makes, from
`Serves`, a claim or the agenda, ends in that fragment.

A template has three functions and no others beyond `html/template`'s own: `first`, which takes a
count and a record list and returns that many records; `date`, which formats a time as
`YYYY-MM-DD`; and `age`, which gives the whole days between two times. The set is closed for the
reason the summary template's is: a function that read the environment or a file would let a module
print a deployment's secrets onto a page.

A template may call the kernel's partials, such as `claims`, `revision`, `record-meta`, `scope` and
`freshness`, each with a `Record`, so a custom layout keeps the kernel's markup for the parts it
does not change.

### Names a template may not define

The kernel's own template names are reserved, because a module template that defined one would
replace the kernel's partial without saying so. The list is computed from the kernel's parsed
templates, so a partial added later is reserved automatically:

`claim`, `claims`, `freshness`, `gauge`, `goal-ref`, `home-body`, `layout-cards`, `layout-list`,
`layout-table`, `memory-list`, `module-body`, `nav`, `page`, `progress`, `record-meta`,
`record-notes`, `render-failure`, `revision`, `scope`, `snoozes`, `sprite`, `state`.

### When a template is wrong

A template that does not parse, or that defines a reserved name, stops the kernel starting, as a
broken summary template does: an author finds out at deployment, not when someone opens a page. A
template that parses and then fails while rendering is replaced, under its heading, by one line,
"The telos goal records could not be shown: the module's view template failed. The kernel log has
the detail." The rest of the page is unaffected, and the log line is `view <module>/<kind>: <error>`.

### The content security policy

Every view response carries `default-src 'none'; style-src 'self'; img-src 'self'; font-src
'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'`. For an author that means a
template cannot run script, cannot carry a `style` attribute or a `<style>` element, cannot load
anything from another origin, and cannot submit a form. Markup is styled by the kernel's classes
and by nothing else. Raw HTML in a record body is omitted, never rendered, so a template that
prints `.Body` prints markdown output only.

### The class contract

A template styles itself with these kernel classes. The stylesheet is the kernel's, and a class
not listed here is not promised to stay.

| class | use |
|---|---|
| `goals` | the list holding a kind's goals (an `ol`) |
| `goal`, `blueprint` | one goal, drawn as a card with four `corner` elements |
| `goal__head`, `goal__id`, `goal__title` | the goal's header row |
| `goal__deadline`, `goal__days` | the `by` date and the days left or past |
| `goal__serves`, `goal__label` | the line naming what the goal serves |
| `claims` | the list of a goal's claims, produced by the `claims` partial |
| `claim`, `claim--pass`, `claim--open`, `claim--behind`, `claim--fail`, `claim--no-evidence`, `claim--unchecked` | one claim, by its state |
| `claim__text`, `claim__meta`, `claim__note` | the claim's quoted text, its source line and its detail |
| `record`, `record--draft` | one record in a card, list or table; `--draft` while unconfirmed |
| `record__title`, `record__key`, `record__body` | a record's heading, a field's label and a field's text |
| `record__meta` | the footer row holding scope, freshness and snoozes |
| `scope`, `scope--global`, `scope--machine`, `scope--project` | where the record applies |
| `freshness`, `freshness--draft`, `freshness--due`, `freshness--unknown` | when it was last confirmed |
| `revision` | a goal's note on what changed |
| `render-failure` | the one-line placeholder; the kernel supplies it |

### Worked example: telos's goal

`modules/telos/module.json` points the `goal` kind at `view/goal.html.tmpl`:

```json
"goal": {"fields": ["id", "title", "ideal", "by"], "optional": ["claims", "serves", "notes"],
         "view": {"template": "view/goal.html.tmpl"}, ...}
```

The template draws each goal as a card. It prints the days to `by` from `.DaysText`, which the
kernel supplies, gives each goal its `.Anchor` as its id, links each name in `.Serves` to the record
it names when one is visible, and leaves claims, revision and the footer to the kernel's partials.

```gotemplate
<ol class="goals">
{{- range .Records}}
<li class="goal blueprint" id="{{.Anchor}}">
  <i class="corner tl"></i><i class="corner tr"></i><i class="corner bl"></i><i class="corner br"></i>
  <header class="goal__head">
    <span class="goal__id">{{.ID}}</span>
    <h3 class="goal__title">{{or .Fields.title .Description}}</h3>
    {{- if .Fields.by}}
    <p class="goal__deadline"><span>by <time datetime="{{.Fields.by}}">{{.Fields.by}}</time></span>{{with .DaysText}}<span class="goal__days">{{.}}</span>{{end}}</p>
    {{- end}}
  </header>
  {{- if .Serves}}
  <p class="goal__serves"><span class="goal__label">Serves</span>{{range .Serves}}{{if .Href}}<a href="{{.Href}}">{{.Name}}</a>{{else}}<span>{{.Name}}</span>{{end}}{{end}}</p>
  {{- end}}
  {{template "claims" .}}
  {{template "revision" .}}
  {{template "record-meta" .}}
</li>
{{- end}}
</ol>
```
