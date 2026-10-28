---
name: trellis-before-dev
description: "Discovers and injects project-specific coding guidelines before implementation begins. Reads the CLAUDE.md key-rules list and the relevant docs/spec/ topic docs (indexed in docs/spec/README.md) for the target area. Use when starting a new coding task, before writing any code, switching to a different area, or needing to refresh project conventions and standards. Note: project-specific routing rules (two-tier split, tiered row limits) are owned by CLAUDE.md「规范沉淀规则」."
---

Read the relevant development guidelines before starting your task.

Execute these steps:

1. **Discover packages and their spec layers**:
   ```bash
   python ./.trellis/scripts/get_context.py --mode packages
   ```

2. **Identify which specs apply** to your task based on:
   - Which package you're modifying (e.g., `cli/`, `docs-site/`)
   - What type of work (backend, frontend, unit-test, docs, etc.)

3. **Read the spec entry points** for this repo:
   ```bash
   cat docs/spec/README.md
   ```
   The CLAUDE.md key-rules list is auto-injected each session — treat it as the pre-development checklist: every row names a rule you must follow, each linking to its `docs/spec/` detail doc.

4. **Read the specific topic docs** linked from the key-rules rows that are relevant to your task (e.g., `cross-layer-contracts.md`, `logging-and-errors.md`, `perf-baseline.md`). The key-rules row is NOT the full contract — it points you to the actual detail doc. Read those files to understand the coding standards and patterns.

5. **Check for area-specific conventions**: if your task touches an area with a dedicated spec doc (terminal, assets, pptx-split, browser-channel, etc.), read that doc's relevant sections before coding.

6. Understand the coding standards and patterns you need to follow, then proceed with your development plan.

This step is **mandatory** before writing any code.
