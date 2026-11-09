# S.I.M.A. — Agent Instructions

## Project Identity
S.I.M.A. is an autonomous mission and fleet coordination platform initially targeting multi-AUV underwater exploration.

## Architecture
- S.I.M.A. is a separate Go project and repository.
- AEGIS is an external Go module dependency.
- S.I.M.A. must never be imported by AEGIS.
- AEGIS owns transport, communication reliability, node health, discovery, and distributed coordination primitives.
- S.I.M.A. owns mission planning, fleet management, vehicle state, task allocation, decision-making, and mission recovery.
- Never duplicate AEGIS transport implementations inside S.I.M.A.
- Never modify the AEGIS repository unless explicitly instructed.

## Engineering Rules
- Use idiomatic, maintainable Go.
- Prefer small interfaces and modular architecture.
- Keep transport-independent mission logic separate from AEGIS integration.
- Avoid unnecessary abstractions and overengineering.
- Add deterministic tests for new functionality.
- Preserve clear package boundaries.

## Development Workflow
- Inspect before implementing.
- Start with read-only architecture analysis.
- Report findings, risks, proposed interfaces, and file structure before writing code.
- Do not implement anything until explicitly approved.
- Work in small, independently testable phases.
- Run gofmt, go test ./..., go test -race ./..., and go vet ./... when applicable.
- Never claim a feature is implemented or validated without evidence.

## Underwater Constraints
- Acoustic communication is low-bandwidth, high-latency, and intermittent.
- Do not assume continuous connectivity.
- Vehicle-level safety and critical control remain onboard.
- Do not claim real acoustic modem support until implemented and tested.
