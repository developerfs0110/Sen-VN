---
name: archify
description: Render clean, layered Mermaid Pro architecture/workflow diagrams and interactive HTML architecture widgets without visual clutter. Use when designing systems, workflows, pipelines, state machines, or visual walkthroughs.
---

# Archify — Clean Workflow & Architecture Diagrams (`archify`)

Create structured, readable diagrams for system architecture, business flows, video/media pipelines, database ERDs, and task DAGs.

## 1. Mermaid Pro Rules (Markdown & Delivery Reports)
1. **Supported Diagram Types Only:** `flowchart TD` / `flowchart LR`, `sequenceDiagram`, `stateDiagram-v2`, `classDiagram`, `erDiagram`.
2. **Layered Subgraphs (`<= 5` Nodes per Subgraph):** Group related components into named `subgraph` blocks instead of dumping flat spaghetti graphs.
3. **Quoted Semantic Node Shapes (Zero `<br/>` Tags):**
   - Always wrap labels in double quotes `"..."` when containing parentheses, slashes, or colons.
   - Avoid HTML `<br/>` inside node labels.
   - Use semantic shapes: `(["Entry / User"])` for actors/triggers, `["Service / Process"]` for logic, `[("SQLite / Storage")]` for data stores, `{"Gate / Decision"}` for checks.
4. **Numbered Edges & Clean Palette:** Label key flow transitions (`①`, `②`, `③`) and apply high-contrast `classDef` styles:
   ```mermaid
   flowchart LR
     subgraph Input["1. Intake & Scope"]
       U(["User Request"]) -->|"① sen intake"| R[("SQLite requirements")]
       R -->|"② sen begin"| T["Scoped Task + Snapshot"]
     end
     subgraph Gate["2. Referee Verification"]
       T -->|"③ sen verify"| V{"Exit 0 & Tests > 0?"}
       V -->|"Pass"| D(["sen done Report"])
       V -->|"Fail (<=3x)"| U2["Fix or sen undo"]
     end
     classDef primary fill:#eef6ff,stroke:#1d4ed8,color:#0f172a,stroke-width:1.5px;
     classDef store fill:#f0fdf4,stroke:#15803d,color:#0f172a,stroke-width:1.5px;
     classDef gate fill:#fffbeb,stroke:#b45309,color:#0f172a,stroke-width:1.5px;
     class T,U2 primary;
     class R store;
     class V gate;
   ```

## 2. Interactive HTML Architecture Widget (When Rich Visuals Are Requested)
- Build a self-contained single-file HTML/SVG preview (`docs/diagrams/<name>.html`) with clickable component cards, zoom/filter toggles, and zero external CDN secrets when the user asks for an interactive visual walkthrough.

