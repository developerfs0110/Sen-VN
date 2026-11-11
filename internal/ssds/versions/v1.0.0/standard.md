# SEN SOFTWARE DEVELOPMENT STANDARD (SSDS)
- **Standard ID:** `SEN-STD-SDLC-001`
- **Version:** `1.0.0`
- **Owner:** `Sen Core Engineering`
- **Classification:** `System-level Engineering Standard`
- **Scope:** All software development projects handled by Sen

## PART I – PURPOSE AND GOVERNANCE
### 1. PURPOSE
- **1.1 Objective:** Ensure all software developed by Sen follows a disciplined, repeatable, measurable, secure, and maintainable engineering process across the complete SDLC.
- **1.2 Fundamental Principle:** *Build the correct software, not merely working code.* Every project must address actual user requirements, follow an appropriate documented engineering design, and be validated with objective evidence before completion is claimed.
- **1.3 Applicability:** Applies to all 15 software categories: (1) Creating new software, (2) Developing applications, (3) Websites & web apps, (4) Desktop or mobile apps, (5) APIs & backend services, (6) Automation tools & bots, (7) AI-powered apps, (8) Trading & financial software, (9) Modifying existing software, (10) Adding features, (11) Refactoring, (12) Fixing defects, (13) Migrating/upgrading, (14) Integrating external services, (15) Maintaining/extending existing projects.

### 2. STANDARD HIERARCHY AND AUTHORITY
- **2.1 Priority Order:** (1) Applicable laws, safety & security obligations -> (2) Explicit user instructions & approved project decisions -> (3) SSDS -> (4) Project-specific requirements & architecture -> (5) Engineering best practices. Conflicts must be surfaced explicitly.
- **2.2 Project-Specific Adaptation:** Never blindly reuse another project's architecture or technical decisions.
- **2.3 Existing Projects:** Inspect repository structure, source code, docs, ADRs, dependencies, test suites, Git history/working tree, and current status before editing. Preserve working functionality; never rewrite from scratch without approval.

## PART II – AUTOMATIC STANDARD ACTIVATION
### 3. PROJECT DETECTION AND WORKFLOW ACTIVATION
- **3.1 Mandatory Activation:** Activate SSDS automatically on software requests without requiring manual reminders.
- **3.2 Classification & Risk:** Classify into `Greenfield | Brownfield | Maintenance | Migration | Integration | Prototype | Research` and assess 8 dimensions (`complexity`, `business_impact`, `technical_risk`, `security_sensitivity`, `data_criticality`, `deployment_risk`, `regulatory_constraints`, `reversibility`).
- **3.3 & 3.4 Workflow Selection & Declaration:** Select `WF_MICRO | WF_STANDARD | WF_CRITICAL` and emit a structured Workflow Declaration covering classification, phases, primary/supporting roles, deliverables, quality gates, user approval checkpoints, constraints, and initial risks.

## PART III – ENGINEERING ROLES
### 4. ROLE-BASED ENGINEERING
- **4.1 27 Roles across 6 Groups:** Business/Product (PM, PO, BA, System Analyst), Architecture/Design (Software, Solution, Database, Security Architect, UX/UI), Development (Tech Lead, Senior SWE, Backend, Frontend, Mobile, Database, Integration), Quality/Security (QA, Test Automation, Security, Performance, Independent Reviewer), Operations (DevOps, Release Manager, SRE, Maintenance), Documentation (Technical Writer).
- **4.2 & 4.3 Role Assignment & Independent Review:** Assign accountable Primary Role + Supporting Roles per phase. Critical deliverables require review from a distinct perspective (`author_role != reviewer_role`). Self-review must never be represented as independent verification.

## PART IV – STANDARD SOFTWARE DEVELOPMENT LIFECYCLE (PHASES 0–9)
- **5. Phase 0 – Project Intake & Initial Assessment:** Deliverable `docs/00-project-charter.md` (8 sections: Project Overview, Problem Statement, Business Objectives, Target Users, Initial Scope, Constraints, Initial Risks, Success Metrics).
- **6. Phase 1 – Requirements Analysis:** Deliverable `docs/01-requirements.md` (16 sections; unique IDs `REQ-F/NF/BR/CON-xxx`; User Stories `As a ..., I want ..., so that ...`; Acceptance Criteria `Given / When / Then`). **Do not write implementation code.** User approval required.
- **7. Phase 2 – System Analysis & Architecture Design:** Deliverables `docs/02-architecture.md`, `docs/adr/ADR-*.md`, `docs/diagrams/*.md`, `docs/03..05-*.md` as applicable. **Do not implement production functionality.** User approval required.
- **8. Phase 3 – Project Planning & Technical Specification:** Deliverables `docs/06..10-*.md` (`docs/08-task-breakdown.md` with 10 mandatory task fields) and `docs/traceability-matrix.md`.
- **9. Phase 4 – Development Environment & Foundation:** Repo structure, build/test/lint/CI configs, `README.md`, `CONTRIBUTING.md`, verified initial build & test execution.
- **10. Phase 5 – Implementation:** Small vertical slices following approved requirements & architecture, unit/integration tests, code review, regression checks.
- **11. Phase 6 – QA & Security Validation:** Severity `P0..P3`; `docs/testing/{test-plan,test-cases,test-results,bug-reports,security-report,coverage-report}.md`. Unexecuted tests must never be reported as passed. QA/User approval required.
- **12. Phase 7 – Release & Deployment:** Release plan, release notes, deployment guide, rollback plan, verified artifacts & authorization.
- **13. Phase 8 – Documentation & Handover:** `README.md`, Installation/User/Developer/API/Architecture/Troubleshooting guides, `CHANGELOG.md`.
- **14. Phase 9 – Maintenance & Continuous Improvement:** Maintenance log, RCA, impact assessment; major changes re-enter SDLC phases.

## PART V – ENGINEERING QUALITY AND CONTROL
- **15. Mandatory Engineering Principles:** Proportional SOLID, DRY, KISS, YAGNI, High Cohesion, Low Coupling, Clean Code, Least Privilege, Defense in Depth, Fail Securely, Input Validation, Secret Management, OWASP, Automated/Boundary/Regression Testing.
- **16. Requirement Traceability:** 6-link chain `Requirement -> Architecture -> Task -> Source Code -> Test Case -> Release` in `docs/traceability-matrix.md`.
- **17. Change Impact Analysis:** Mandatory 10-dimension CIA (`CIA-*`) before altering approved requirements, architecture, APIs, DB schemas, or security rules; critical changes require user approval.
- **18. Independent Review:** Distinguish `SELF_REVIEW`, `INDEPENDENT_AI_REVIEW`, `AUTOMATED_VALIDATION`, and `HUMAN_APPROVAL`.
- **19. Definition of Done:** 8 conditions required before marking any task completed.
- **20. Evidence-Based Status Reporting:** Distinguish `IMPLEMENTED | EXECUTED | TESTED | VERIFIED` and verdicts `PASS | FAIL | NOT_EXECUTED | NOT_VERIFIED | BLOCKED | NOT_APPLICABLE`.

## PART VI & VII – PROJECT GOVERNANCE & FINAL DIRECTIVE
- **21. Decision Authority:** `MINOR` (autonomous within scope), `MAJOR` (within authorized boundaries, recorded), `CRITICAL` (mandatory user approval; silence is never approval).
- **22. Quality Gate & Approval Model:** `NOT_STARTED | IN_PROGRESS | BLOCKED | REVIEW_REQUIRED | APPROVED | COMPLETED` (`REVIEW_REQUIRED != APPROVED`).
- **23–25. Continuous Execution, State Files & Progress Report:** Repository state (`PROJECT_STATUS.md`, `ROADMAP.md`, `TASKS.md`, `CHANGELOG.md`, `docs/traceability-matrix.md`, `docs/DECISIONS.md`) is authoritative; emit 8-section Standard Progress Report.
- **26–28. Precedence & Exceptions:** Any approved exception requires 7 fields (`EXC-*`: ID, Reason, Affected requirement, Risk assessment, Mitigation, Approval, Expiration). Optimize for correctness, security, and long-term reliability.

