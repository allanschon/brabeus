# Naming the system — research and a proposed set, for later review

Written 2026-09-26, against the specification at v1.4. **Nothing is decided, registered or
renamed.** The operator's first reaction on reading the Greek set is recorded at the end, and
this note exists so the question can be picked up later without re-running the checks.

## What needs a name

The spec uses descriptive names throughout. The components that would benefit from a proper
name, and the relationship a good set of names should preserve:

| component | today | relates to the others as |
|---|---|---|
| the whole system | "the personal context system" | the thing a person points at |
| the kernel | "the kernel", the kernel's repository | what everything stands on; has no opinions |
| the `working-memory` profile, and so the `memory` module | descriptive | the assistant's own notes — model-written, searched |
| the `ratified-record` profile, and so the four core modules | descriptive | the person's own record — confirmed, rendered |
| the interview | `/interview` | the examination that keeps the record true |
| claims and their evidence | "claims" | testimony the record calls |
| the view | "the view" | where the whole is seen at once |

The individual modules — `memory`, `identity`, `telos`, `health`, `finance` — keep their
descriptive names by decision. Names hang on the **profiles** rather than on "the working memory
module" and "the personal context core" because the profile is the actual architectural
distinction (spec §1.1): "an X module" and "a Y module" then say exactly what the manifest says,
and a third profile gets a third name.

## Criteria

1. Pronounceable by someone who does not speak the source language, with the stress obvious.
2. No overlap with an existing open-source or commercial project, especially in AI memory.
3. The set relates to itself the way the components do, and admits new members.
4. The whole system's name is available as a domain.
5. Not so esoteric that the name needs explaining every time — the criterion the first round
   underweighted.

## Method

Collision: web and GitHub search on each candidate with software/AI/memory qualifiers.
Domains: RDAP against the registries directly — Verisign for `.com` and `.net`, Google for
`.dev` and `.app`, Identity Digital for `.io`, PIR for `.org` — with a known-free and a
known-taken control on each. RDAP answers *registered or not*; it does not show a registry's
premium pricing, which appears only at checkout. Checked on 2026-09-26.

## Finding 1: the memory words are gone

Every Greek word that means memory is taken by an AI project, usually several: `Mneme`,
`Anamnesis` (four), `Hypomnema` (a structured memory for Claude Code — a direct collision),
`Hexis` (three agent frameworks). So are the Socratic words: `Elenchus` (five), `Gnothi` (an AI
self-discovery journal), `Basanos` (an MCP server), `Peira` (three). Also `Skopos`, `Endoxa`,
`Deltos`, `Naos`, `Krepis`, `Ephemeris`, `Thesauros`. The words that survived are the ones a
memory-project founder would not reach for first.

## Finding 2: single dictionary words have no `.com`, in any language

Every Greek, Latin and English word checked is registered on `.com`, and the English ones on
every other TLD as well: `plumbline`, `truing`, `keelson`, `reckoning`, `commonplace`, `daybook`,
`sounding`, `bearings`, `tabularium`. So are the English coinages tried — `heldtrue` (a
fact-checker), `keelmark` (three companies), `owncourse`, `plumbtrue`, `stillpoint`, `truekeel`,
`heldfast`, `selfledger`, `mirrorwell`, `keelwise`. English's good short words for this were taken
by 2010. The reason Greek works is not that it is more apt; it is that the pool of pronounceable,
meaningful, unclaimed words is larger. Latin, the traditional naming language of Western products,
is more mined than Greek.

A two-word name — a brand word plus a generic — remains the way to a `.com` if one is ever
wanted. It was not pursued.

## The Greek set

The family is "the examined life": each name is what the component does to the person's record.

| component | name | say it | meaning | collisions | domains free |
|---|---|---|---|---|---|
| system | **Katoptron** | ka-TOP-tron | κάτοπτρον, *mirror* — the system reflects the gap back | none: a flashlight fish, a game item, the dictionary | `.dev` `.app` `.io` `.net` `.org`; `.com` registered 2024-11, **expires 2026-11-25** |
| kernel | **Themelion** | theh-MEH-lee-on | θεμέλιον, *foundation stone* — stands under everything, opines on nothing | none | `.io` only; a kernel needs no domain |
| `working-memory` | **Apotheke** | ah-po-THEH-kee | ἀποθήκη, *storehouse*; root of *apothecary* and *boutique* | none in software; the German word for pharmacy | none; a profile needs no domain |
| `ratified-record` | **Oikeion** | oy-KAY-on | οἰκεῖον, *what is one's own*; the Stoic term for recognising what belongs to you | none | `.dev` `.app` `.io` `.net` `.org` |
| interview | **Exetasis** | ex-EH-ta-sis | ἐξέτασις, *examination*; Socrates' word — the unexamined life is *anexétastos bíos* | none | `.dev` `.app` `.io` |
| claims | **Martyria** | mar-tee-REE-a | μαρτυρία, *testimony* — a claim calls witnesses | none | `.dev` `.io` |
| view | **Skopia** | sko-PEE-a | σκοπιά, *lookout* | none | none |
| reserved: third profile | **Koinon** | KOY-non | κοινόν, *the common thing* — a memory shared across people | none | none |

Alternatives checked and not preferred: **Idion** (ἴδιον, *one's own*, ID-ee-on; root of
*idiom*) for the ratified profile — easier to say than Oikeion, not yet domain-checked;
**Adyton** (the innermost room, "not to be entered" — `audience: self` in one word) — `.app`
free only; **Stylobate** (the platform columns stand on) for the kernel — an English word, one
dormant repository; **Temenos** for the system — likely collisions, unchecked.

A second Greek family, the temple — Stylobate for the kernel, columns for modules, Adyton for the
ratified record, Apotheke for the storeroom, Temenos for the whole — relates physically and reads
well as a metaphor, and is a weaker set of names: two unchecked, and the sentence is better than
the word.

## The Latin pass

Tried second, at the operator's request, on the hope that Latin would be less esoteric because
so much of it is already half-English. It is; it is also far more taken.

| candidate | meaning | collisions | domains |
|---|---|---|---|
| **Recognitio** (rek-og-NIT-ee-o) | the censors' annual review of the knights: each rode past, was examined, and kept his horse or was struck off — the interview and the claims in one word | none found | `.dev` `.app` `.io` free |
| **Perpendiculum** (per-pen-DIK-yoo-lum) | the plumb line — what you hold the record against | none found | `.dev` `.app` `.io` `.net` free |
| Examen | the Ignatian daily review of the day against one's values — the best meaning of all | "exam" in three languages | taken everywhere |
| Adversaria | a Roman waste-book of rough daily notes — exactly a working memory | a Claude Code critical-thinking plugin, an agent platform | `.dev` only |
| Proprium | *one's own* — the Latin Oikeion | none found | `.dev` only |
| Liquet | *it is clear* | none found | `.dev` `.io` |
| Ratum, Cardo, Probatio, Conspectus, Specula, Constat, Solum, Regula | ratified · hinge · proof · overview · watchtower · it stands · ground · rule | Cardo (a fintech), Probatio, Specula (four) | taken everywhere |

Two Latin words earn a place on the shortlist on meaning: **Recognitio** for the interview (or
for the system, if the interview is what the system *is*), and **Perpendiculum** for the
system. Both are five syllables, which is the esoteric problem again in a different alphabet.

## The mirror and the honest judge — a second pass, 2026-09-26

The operator liked the themes of reflection, the mirror, an honest judge, an arbiter, and asked
for those threads to be chased across languages. Same method as above.

### Mirror

| candidate | say it | meaning | collisions | domains free |
|---|---|---|---|---|
| **Esoptron** | es-OP-tron | ἔσοπτρον, the New Testament's mirror — *"through a glass, darkly"* (1 Cor 13:12, *di' esoptrou en ainigmati*) | a pharmaceutical QMS consultancy on LinkedIn; no software | `.dev` `.app` `.io` |
| Enoptron | en-OP-tron | ἔνοπτρον, mirror; the rarer form | none | `.dev` `.app` `.io` |
| Anaklasis | a-NAK-la-sis | ἀνάκλασις, reflection of light | an X-ray reflectometry package | `.dev` `.app` `.io` `.org` |
| Dioptra | die-OP-tra | the sighting instrument | NIST's AI test platform | taken everywhere |
| Adarsha | ah-DAR-sha | Sanskrit आदर्श: *mirror* — and, in the same word, *ideal* | none in software; a common given name (Adarsh) | `.app` `.io` |
| Kagami | ka-GA-mi | 鏡, mirror; one of the three sacred treasures | a Microsoft document scanner | taken everywhere |
| Skuggsjá | — | Old Norse "shadow-see", the *King's Mirror* | a Claude Code retrospective tool — of course | — |
| Silvering · Tain | — | the silver backing that makes glass reflect · the tin behind a mirror | none | `.dev` / none |

`Adarsha` deserves a note: a single word that means both *mirror* and *ideal* is almost the
system's definition. It is also someone's name in a large part of the world, which is a reason
not to use it, and only two TLDs are open.

### Honest judge, arbiter

| candidate | say it | meaning | collisions | domains free |
|---|---|---|---|---|
| **Brabeus** | BRAB-yoos | βραβεύς, the umpire at the games: judges honestly, awards the prize. Root of *brabeion*, the prize Paul presses toward, and *brabeuō*, to arbitrate | **none** — no company, product, repository or trademark found | **`.com` `.net` `.org` `.dev` `.app` `.io` — all six** |
| Brabeutes | bra-BYOO-tees | βραβευτής, the same office, agent-noun form | none | all six |
| Euthyna | YOO-thi-na | εὔθυνα, the audit an Athenian official faced on leaving office — literally "straightening"; *held to account* in one word | an AI code-audit tool and an ESG company | `.app` `.io` |
| Rhadamanthys | rad-a-MAN-this | the incorruptible judge of the dead; *rhadamanthine* = rigorously just | none in software | `.dev` `.app` |
| Deemster | DEEM-ster | the title of judges on the Isle of Man; from Old English *dēman*, to judge, to *deem* | none in software | `.dev` `.app` `.org` |
| Assayer | as-SAY-er | who tests ore for what it is really worth | two projects, one an audit platform | taken |
| Fair Witness | — | Heinlein's trained observer who reports only what is seen | some | `.dev` |
| Verax · Probus · Trutina · Aequus · Judex | — | Latin: truthful · upright · the balance · fair · judge | Verax (two AI companies), Probus (a company) | taken everywhere |
| Psephos · Krites · Dikastes | — | the voting pebble · the judge · the juror | Psephos (a company, two projects) | partial |

### What the second pass changes

**Brabeus is the strongest candidate found in any pass.** It is short, two syllables, stressed on
the first, spelt as it sounds; its meaning is the honest umpire who watches the contest and
awards the prize — which is what a system that checks your goals against evidence and reflects
the result back is for; nothing in software or commerce uses it; and every domain is open,
including `.com`. The only thing it lacks is familiarity, and it is no more unfamiliar than
*Themelion* or *Katoptron*, and easier to say than either.

It also sits naturally with the mechanism names already chosen, which turn out to be a
courtroom: **Martyria** is the testimony the claims call; **Exetasis** is the examination; the
umpire hears both and rules. The mirror thread then names the view — **Esoptron**, the glass you
look into — rather than the system, which reads better: the system judges, the view reflects.

A revised proposed set, for sitting with beside the first:

| component | first set | second set |
|---|---|---|
| system | Katoptron | **Brabeus** |
| kernel | Themelion | Themelion |
| `working-memory` profile | Apotheke | Apotheke |
| `ratified-record` profile | Oikeion | Oikeion (or Idion) |
| interview | Exetasis | Exetasis |
| claims | Martyria | Martyria |
| view | Skopia | **Esoptron** |
| reserved: third profile | Koinon | Koinon |

`brabeus.com` is unregistered as of 2026-09-26. Of everything in this note, it is the one
address worth holding before deciding anything else, because a two-syllable Greek word with a
free `.com` will not stay that way.

## The proposed set, as it stood after the first pass

Katoptron · Themelion · Apotheke · Oikeion · Exetasis · Martyria · Skopia, with Koinon reserved
and the modules keeping their names. `katoptron.dev` or `.io` for the system. The second pass
above proposes Brabeus for the system and Esoptron for the view, everything else unchanged.

## The recommended scheme: one name, descriptive components

Proposed by the operator after the second pass, 2026-09-26: find one good name and call the
components *X Kernel*, *X Memory*, *X Personal Context*, rather than giving each a name of its
own. This is recommended over both Greek sets, for reasons that are structural rather than
aesthetic:

- All the weight rests on one word, which is the only one that has to be good — and the second
  pass found one.
- Descriptive component names relate to each other *literally*, which is stronger than
  analogously: the names teach the architecture. "An X module" under a profile called
  `working-memory` needs no gloss.
- It matches the spec's shape exactly: one kernel, two products on it (§1.1), a thin plugin.
- The mechanisms already have plain names the plain-language description uses without strain.
- Criterion 5 is met by construction: one unfamiliar word, explained once.

With Brabeus as the word:

| thing | name |
|---|---|
| the project, the repository, the domain | **Brabeus** |
| the server | **Brabeus Kernel** — binary `brabeus` |
| the working-memory product: today's server, the `memory` module | **Brabeus Memory** |
| the personal-record product: the four core modules | **Brabeus Personal Context** |
| the assistant-side client | **Brabeus plugin** for Claude Code; another harness gets its own |
| profiles | `working-memory`, `ratified-record` — unchanged, lowercase, in manifests |
| mechanisms | the interview · claims · review · the view — unchanged |
| modules | `memory` · `identity` · `telos` · `health` · `finance` — unchanged |

"AI" is deliberately absent from "Brabeus Memory": it adds nothing and will date.

The Greek sets above stay in this note as the research that found the word, not as names in
waiting. If Brabeus is ever rejected, the search resumes from the second-pass shortlist —
Esoptron, Deemster, Oikeion — under this same scheme, not from the families.

## Coined brands — a third pass, 2026-09-26

The operator's reservation about Brabeus: as a brand it would need building, whereas *LifeOS*
told you what it was before you read a word — and even so its makers ended up at
`ourlifeos.ai` for the web. So: coinages that carry a hint, with the domain allowed to
compromise.

Method as before, plus `.ai` (Identity Digital's RDAP, controls pass). Registration details read
for the taken `.com`s, because a parked squat and a business are different obstacles.

| candidate | the hint | collisions | `.com` | free on |
|---|---|---|---|---|
| **Selfkeep** | *keep*: the innermost stronghold of a castle, the last room an attacker reaches — `audience: self` — and *to keep*: to maintain, to hold true, to keep a record | none: neighbours are *shelfkeep*, *ownkeep*, *kept* (all self-hosted notes apps), none a collision | parked since 2019 at NameBright, expires 2027-05 | `.io` `.dev` `.app` `.net` `.org` `.ai` |
| **Mirrorkeep** | the keep that holds the mirror — both threads in one word | none: an OpenSCAD function name | registered 2026-09-13 at Cloudflare — someone had the idea two weeks ago | `.io` `.dev` `.app` `.net` `.org` `.ai` |
| **Telograph** | *telos* written down, on the pattern of *autograph*; rhymes with *telegraph*, so it says itself | a band on SoundCloud | held since 2005 | `.io` `.dev` `.app` `.net` `.org` `.ai` |
| **Selfsworn** | a record you swore to — ratification, testimony, the courtroom in plain English | none | registered 2026-07 at Porkbun | `.io` `.dev` `.app` `.net` `.org` `.ai` |
| **Selfwitness** | Heinlein's *fair witness*, turned on yourself | none | parked since 2023 at NameBright, expires 2026-09-30 | `.io` `.dev` `.app` `.net` `.org` `.ai` |
| Telokeep | *telos* + *keep*; the hint is weaker because *telo-* reads as *telomere* to most | none | **free** | everything |
| Selfvouch | to vouch for yourself — which is the wrong direction; the system vouches, or refuses to | none | **free** | everything |
| Teloscope · Truekeep · Selfscope · SelfOS · Truestate · ContextOS | an instrument for seeing purpose · the keep of truth · — · — · the honest current state · — | a Claude Code metrics tool · a company · an executive-coaching firm · taken · real estate · taken | taken | — |

For reference on the compromise: `lifeos.ai` and `ourlifeos.ai` are both registered;
`brabeus.ai` is free, alongside its six other TLDs.

### How the coinages compare with Brabeus

Three are real candidates: **Selfkeep**, **Telograph**, **Selfsworn**.

*Selfkeep* is the strongest brand of the three. Two syllables, two English words, and both
halves are load-bearing: *self* is the subject, *keep* is simultaneously the stronghold nobody
else may enter and the verb for maintaining a record. It sounds like a product without sounding
like a startup. Its `.com` is a parked squat, buyable or waitable; everything else is open,
including `.ai`. Its weakness is company: the self-hosted notes apps next door (*shelfkeep*,
*ownkeep*, *kept*) mean it will read as "another notes app" to someone who skims.

*Telograph* carries the most specific hint — it names the actual idea, purpose written down —
and it rhymes with a word everyone knows, so it is pronounced right on first sight. It is one
letter from *telegraph*, which is either memorable or confusing, and the `.com` has been held by
someone since 2005.

*Selfsworn* has the best meaning for what the system actually does — a record you swore to,
which the profiles enforce — and the weakest surface: it sounds like a legal term, and its
`.com` was registered this July.

Against all three, *Brabeus* has one advantage they cannot match — every domain, including
`.com`, open — and one disadvantage they all share in reverse: it means nothing to anyone until
told. The operator's framing is the right one: a coined brand starts with a hint and a domain
compromise; a Greek brand starts with a clean domain and a story to tell every time.

### Two-word forms, since the scheme allows them

Under the one-name scheme (above), the components are *X Kernel*, *X Memory*, *X Personal
Context*. Read aloud: "Selfkeep Kernel", "Selfkeep Memory", "Selfkeep Personal Context";
"Telograph Kernel", "Telograph Memory"; "Brabeus Kernel", "Brabeus Memory". All three work;
*Selfkeep Memory* and *Selfkeep Personal Context* are the ones that sound like they already
exist.

## The operator's reaction, and what is open

On first reading (2026-09-26): the meanings are liked; the words are more esoteric than
expected, and Katoptron "doesn't exactly roll off the tongue." After the second pass, the
operator proposed the one-name scheme above; it is recorded as the recommendation. After the
third pass the leading candidates are **Brabeus** (clean domains, no hint) and **Selfkeep** (a
hint, a squatted `.com`), with Telograph and Selfsworn behind them. Still to be sat with.
Nothing is registered.

What would change the answer:

- **A shorter, plainer umbrella.** The one name people will say most is the one that most needs
  to be easy. Everything below it can afford to be a term of art, the way `kernel` and
  `profile` already are. If Katoptron falls, the profile and mechanism names can stay.
- **A two-word umbrella** (brand word + generic) if a `.com` ever matters.
- **Checking Idion**, which may replace Oikeion on pronounceability alone.
- **`katoptron.com` expiring 2026-11-25.** A lapsed registration sometimes drops; worth a look
  in December if the name survives.

Nothing here has been registered. Registering a domain is the one action with a clock on it,
and it is the operator's to take.
