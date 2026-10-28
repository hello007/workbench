---
name: trellis-update-spec
description: "Captures executable contracts and coding conventions into docs/spec/ documents and the CLAUDE.md key-rules list. Use when learning something valuable from debugging, implementing, or discussion that should be preserved for future sessions."
---

# Update Code-Spec - Capture Executable Contracts

When you learn something valuable (from debugging, implementing, or discussion), use this to update the relevant spec documents.

> **项目权威声明（本仓定制覆盖）**：项目特定口径——两级分流规则、钩子级两档上限（普通行 ≤250 / 钩子密集行与合并行 ≤350）、写入路径（`docs/spec/<topic>.md` 与 CLAUDE.md 关键规则清单）——**以 CLAUDE.md「规范沉淀规则」节为唯一权威**。本 skill 只提供通用方法论；两者冲突时以 CLAUDE.md 为准。分档细则见 [docs/spec/README.md](../../docs/spec/README.md)。

**Timing**: After completing a task, fixing a bug, or discovering a new pattern

---

## Code-Spec First Rule (CRITICAL)

In this project, "spec" for implementation work means **code-spec**:
- Executable contracts (not principle-only text)
- Concrete signatures, payload fields, env keys, and boundary behavior
- Testable validation/error behavior

If the change touches infra or cross-layer contracts, code-spec depth is mandatory.

### Mandatory Triggers

Apply code-spec depth when the change includes any of:
- New/changed command or API signature
- Cross-layer request/response contract change
- Database schema/migration change
- Infra integration (storage, queue, cache, secrets, env wiring)

### Mandatory Output (7 Sections)

For triggered tasks, include all sections below:
1. Scope / Trigger
2. Signatures (command/API/DB)
3. Contracts (request/response/env)
4. Validation & Error Matrix
5. Good/Base/Bad Cases
6. Tests Required (with assertion points)
7. Wrong vs Correct (at least one pair)

---

## When to Update Specs

| Trigger | Example | Target |
|---------|---------|--------|
| **Implemented a feature** | Added a new integration or module | Relevant `docs/spec/<topic>.md` |
| **Made a design decision** | Chose extensibility pattern over simplicity | Relevant topic doc + "Design Decisions" section |
| **Fixed a bug** | Found a subtle issue with error handling | Relevant topic doc (e.g., error-handling sections) |
| **Discovered a pattern** | Found a better way to structure code | Relevant topic doc |
| **Hit a gotcha** | Learned that X must be done before Y | Topic doc + CLAUDE.md key-rules line (hook-level rule) |
| **Established a convention** | Team agreed on naming pattern | Relevant topic doc |
| **New must-know rule for future coding** | A ban, a single-point function name, an error code, a sync point | CLAUDE.md key-rules list (hook-level, per tiered limits) |

**Key Insight**: Spec updates are NOT just for problems. Every feature implementation contains design decisions and contracts that future AI/developers need to execute safely.

---

## Where to Write: Two-Tier Routing (this repo)

This repo routes captured knowledge into **two tiers** (authoritative source: CLAUDE.md「规范沉淀规则」; details in [docs/spec/README.md](../../docs/spec/README.md)):

| Tier | Destination | Content |
|------|-------------|---------|
| **Detailed contract / topic doc** | `docs/spec/<topic>.md` | Cross-layer contracts, signature-sync rules, anything needing full context |
| **Hook-level key rule** | CLAUDE.md key-rules list (one table row) | Single-point function names, bans, error codes, sync points; row links to its `docs/spec/` detail doc |

**Sinking criterion** (下沉判据):
- "Next time you write code, follow this" (bans, single-point names, error codes, sync points) → stays inline in the key-rules row
- "Why it exists / how the bug happened" (mechanism, date, incident history, signature evolution) → sinks into `docs/spec/`

**Tiered row limits** (硬顶): normal rows ≤250 chars; hook-dense rows (≥7 hooks) and phase/merge rows ≤350. A new row exceeding its limit must sink the excess detail into `docs/spec/` on the spot — never land the row first and clean up later.

**No `guides/` tier in this repo**: thinking-checklist content that the upstream framework routes to `guides/*.md` has no separate home here; fold it into the relevant topic doc or the key-rules row instead.

---

## Update Process

### Step 1: Identify What You Learned

Answer these questions:

1. **What did you learn?** (Be specific)
2. **Why is it important?** (What problem does it prevent?)
3. **Where does it belong?** (Which tier — topic doc or key-rules row — and which file?)

### Step 2: Classify the Update Type

| Type | Description | Action |
|------|-------------|--------|
| **Design Decision** | Why we chose approach X over Y | Add to "Design Decisions" section of the topic doc |
| **Project Convention** | How we do X in this project | Add to relevant section with examples |
| **New Pattern** | A reusable approach discovered | Add to "Patterns" section |
| **Forbidden Pattern** | Something that causes problems | Add to "Anti-patterns" or "Don't" section |
| **Common Mistake** | Easy-to-make error | Add to "Common Mistakes" section |
| **Convention** | Agreed-upon standard | Add to relevant section |
| **Gotcha** | Non-obvious behavior | Add warning callout |
| **Hook-level rule** | Must-know ban / single point / error code / sync point | Add a key-rules row in CLAUDE.md (within tiered limits) |

### Step 3: Read the Target Spec

Before editing, read the current spec to:
- Understand existing structure
- Avoid duplicating content
- Find the right section for your update

```bash
cat docs/spec/<topic>.md
```

When adding a key-rules row to CLAUDE.md, first read the existing rows in the same category and the linked `docs/spec/` detail doc.

### Step 4: Make the Update

Follow these principles:

1. **Be Specific**: Include concrete examples, not just abstract rules
2. **Explain Why**: State the problem this prevents
3. **Show Contracts**: Add signatures, payload fields, and error behavior
4. **Show Code**: Add code snippets for key patterns
5. **Keep it Short**: One concept per section
6. **Respect tiered limits**: key-rules rows within 250/350 chars; excess detail sinks to the topic doc

### Step 5: Keep Index and Links Consistent (if needed)

- New topic doc → register it in the relevant index (e.g., `docs/spec/README.md` or the doc table in CLAUDE.md)
- New key-rules row → the row must link to its `docs/spec/` detail doc

---

## Update Templates

### Mandatory Template for Infra/Cross-Layer Work

```markdown
## Scenario: <name>

### 1. Scope / Trigger
- Trigger: <why this requires code-spec depth>

### 2. Signatures
- Backend command/API/DB signature(s)

### 3. Contracts
- Request fields (name, type, constraints)
- Response fields (name, type, constraints)
- Environment keys (required/optional)

### 4. Validation & Error Matrix
- <condition> -> <error>

### 5. Good/Base/Bad Cases
- Good: ...
- Base: ...
- Bad: ...

### 6. Tests Required
- Unit/Integration/E2E with assertion points

### 7. Wrong vs Correct
#### Wrong
...
#### Correct
...
```

### Adding a Design Decision

```markdown
### Design Decision: [Decision Name]

**Context**: What problem were we solving?

**Options Considered**:
1. Option A - brief description
2. Option B - brief description

**Decision**: We chose Option X because...

**Example**:
\`\`\`typescript
// How it's implemented
code example
\`\`\`

**Extensibility**: How to extend this in the future...
```

### Adding a Project Convention

```markdown
### Convention: [Convention Name]

**What**: Brief description of the convention.

**Why**: Why we do it this way in this project.

**Example**:
\`\`\`typescript
// How to follow this convention
code example
\`\`\`

**Related**: Links to related conventions or specs.
```

### Adding a New Pattern

```markdown
### Pattern Name

**Problem**: What problem does this solve?

**Solution**: Brief description of the approach.

**Example**:
\`\`\`
// Good
code example

// Bad
code example
\`\`\`

**Why**: Explanation of why this works better.
```

### Adding a Forbidden Pattern

```markdown
### Don't: Pattern Name

**Problem**:
\`\`\`
// Don't do this
bad code example
\`\`\`

**Why it's bad**: Explanation of the issue.

**Instead**:
\`\`\`
// Do this instead
good code example
\`\`\`
```

### Adding a Common Mistake

```markdown
### Common Mistake: Description

**Symptom**: What goes wrong

**Cause**: Why this happens

**Fix**: How to correct it

**Prevention**: How to avoid it in the future
```

### Adding a Gotcha

```markdown
> **Warning**: Brief description of the non-obvious behavior.
>
> Details about when this happens and how to handle it.
```

---

## Interactive Mode

If you're unsure what to update, answer these prompts:

1. **What did you just finish?**
   - [ ] Fixed a bug
   - [ ] Implemented a feature
   - [ ] Refactored code
   - [ ] Had a discussion about approach

2. **What did you learn or decide?**
   - Design decision (why X over Y)
   - Project convention (how we do X)
   - Non-obvious behavior (gotcha)
   - Better approach (pattern)

3. **Would future AI/developers need to know this?**
   - To understand how the code works → Yes, update spec
   - To maintain or extend the feature → Yes, update spec
   - To avoid repeating mistakes → Yes, update spec
   - Purely one-off implementation detail → Maybe skip

4. **Which tier does it belong to?**
   - [ ] Detailed contract needing full context → `docs/spec/<topic>.md`
   - [ ] Must-know inline rule (ban / single point / error code / sync point) → CLAUDE.md key-rules list
   - [ ] Backend code
   - [ ] Frontend code
   - [ ] Cross-layer data flow
   - [ ] Quality/testing

---

## Quality Checklist

Before finishing your spec update:

- [ ] Is the content specific and actionable?
- [ ] Did you include a code example?
- [ ] Did you explain WHY, not just WHAT?
- [ ] Did you include executable signatures/contracts?
- [ ] Did you include validation and error matrix?
- [ ] Did you include Good/Base/Bad cases?
- [ ] Did you include required tests with assertion points?
- [ ] Is it in the right tier (topic doc vs key-rules row)?
- [ ] Key-rules row within its tiered limit (250/350), excess sunk to the topic doc?
- [ ] Does it duplicate existing content?
- [ ] Would a new team member understand it?

---

## Relationship to Other Commands

```
Development Flow:
  Learn something → /trellis:update-spec → Knowledge captured
       ↑                                  ↓
  /trellis:break-loop ←──────────────────── Future sessions benefit
  (deep bug analysis)
```

- `/trellis:break-loop` - Analyzes bugs deeply, often reveals spec updates needed
- `/trellis:update-spec` - Actually makes the updates
- `/trellis:finish-work` - Reminds you to check if specs need updates, and runs the tiered row-limit self-check before archiving

---

## Core Philosophy

> **Specs are living documents. Every debugging session, every "aha moment" is an opportunity to make the implementation contract clearer.**

The goal is **institutional memory**:
- What one person learns, everyone benefits from
- What AI learns in one session, persists to future sessions
- Mistakes become documented guardrails
