# S.I.M.A. — GitHub Copilot Project Instructions

## 1. Project Identity

S.I.M.A. (Star Intelligent Mission Architecture) is an autonomous mission and fleet coordination platform developed by StarkTech in Go.

Its initial product focus is multi-AUV underwater exploration.

S.I.M.A. is responsible for higher-level autonomous mission management, fleet coordination, vehicle state management, task allocation, decision-making, and mission recovery.

S.I.M.A. is a separate project and repository from AEGIS.

## 2. Source-of-Truth Rules

- The actual source code of S.I.M.A. and AEGIS is authoritative.
- Never assume a feature exists because it is described in documentation.
- Inspect actual APIs, types, tests, and module definitions before designing integrations.
- Do not invent AEGIS functions, interfaces, or capabilities.
- Clearly distinguish implemented features, planned features, and recommendations.

## 3. Relationship with AEGIS

AEGIS is an independent external Go module dependency.

- AEGIS owns communication infrastructure, transports, packet handling, node health, discovery, leader election, failover, and communication-related coordination primitives.
- S.I.M.A. owns mission planning, fleet management, vehicle state, task allocation, mission decisions, and mission recovery.
- S.I.M.A. must integrate with AEGIS through its actual public APIs.
- Never duplicate AEGIS transport implementations.
- Never import S.I.M.A. into AEGIS.
- Never modify the AEGIS repository unless explicitly instructed.

Keep mission logic independent of the underlying transport implementation.

## 4. Intended Architecture

Evaluate and develop the following logical components:

1. Fleet Manager
2. Vehicle Identity and State Manager
3. Mission Planner
4. Task Allocation Engine
5. Decision Engine
6. Coordination Manager
7. Mission Recovery Manager
8. AEGIS Integration Adapter
9. ROS 2 Integration Bridge
10. Optional AI Assistant

These are architectural responsibilities, not a claim that all components already exist.

Prefer modular packages and small interfaces. Avoid creating unnecessary abstractions.

## 5. Autonomous Vehicle Safety

- Critical vehicle control remains onboard each AUV.
- S.I.M.A. must not replace onboard flight, navigation, diving, or safety controllers.
- Vehicles must not depend on continuous communication with the central mission system.
- During communication loss, vehicles must continue only according to their locally implemented safe mission policies.
- When communication returns, vehicle state and mission information can be synchronized and reconciled.
- Mission decisions must account for stale, missing, delayed, and conflicting vehicle information.
- Never assume that a communication timeout automatically means a vehicle has failed.

## 6. Deployment Architecture

S.I.M.A. must support offline and disconnected deployments.

The central mission system may operate on:

- A research vessel
- A local shipboard server
- A shore station
- An isolated research-center network

Continuous internet access must never be a system requirement.

The architecture must tolerate intermittent communication between the central system and AUVs.

## 7. Mission Logic and AI

- Mission-critical decisions must be deterministic, explainable, and testable.
- Safety thresholds, task allocation rules, and recovery policies must not depend on an LLM.
- AI/LLM integration is optional and must remain separate from the mission-critical execution path.
- An AI assistant may support operator interaction, report analysis, and recommendations.
- Do not introduce multiple AI models or unnecessary AI dependencies into the initial prototype.

## 8. Underwater Communication Constraints

Underwater acoustic communication may involve:

- Low bandwidth
- High and variable latency
- Packet loss
- Intermittent connectivity
- Network partitions
- Delayed synchronization

Do not assume that ordinary TCP, UDP, RUDP, or QUIC support automatically provides underwater acoustic modem compatibility.

Do not claim acoustic modem integration or real underwater operation until implemented and tested.

Future DTN and store-carry-forward capabilities must be evaluated separately rather than assumed to exist.

## 9. ROS 2 Integration

ROS 2 is an integration target, not a replacement for S.I.M.A. or AEGIS.

- Keep ROS 2 integration behind a clearly defined bridge or adapter.
- Do not couple core mission logic directly to ROS 2 internals.
- Preserve compatibility with different vehicle platforms where practical.
- Do not assume access to vehicle hardware or ROS 2 packages without verification.

## 10. Go Engineering Standards

- Use idiomatic, maintainable Go.
- Prefer standard-library solutions when appropriate.
- Use small, meaningful interfaces.
- Use context-aware operations.
- Define goroutine ownership and shutdown behavior.
- Prevent data races, goroutine leaks, and unbounded resource usage.
- Add deterministic unit and integration tests.
- Keep package responsibilities clear.
- Avoid unnecessary dependencies and overengineering.

## 11. Development Workflow

Before implementation:

- Inspect the existing repository structure.
- Read relevant source code, tests, go.mod, and documentation.
- Verify the actual AEGIS public API.
- Explain the proposed architecture and integration boundaries.
- Identify risks and missing capabilities.

Do not implement features during a read-only architecture review.

When implementation is explicitly requested:

- Work in small, independently testable phases.
- Make the smallest coherent change.
- Add or update tests.
- Run gofmt, go test ./..., go test -race ./..., and go vet ./... when applicable.
- Report exactly what changed and which checks were actually executed.

Never claim a feature is implemented or validated without evidence.

## 12. Initial Product Priority

The first validation target is a simulation containing three autonomous underwater vehicles.

The intended scenario:

- S.I.M.A. coordinates a multi-AUV exploration mission.
- AEGIS provides the communication and coordination integration.
- One AUV temporarily loses communication.
- The disconnected AUV continues its onboard safe mission policy.
- Other vehicles continue their assigned tasks.
- After reconnection, available state and mission information are synchronized.
- S.I.M.A. evaluates the fleet state and updates task allocation when necessary.

This is a target scenario, not an implemented capability.

Prioritize the smallest architecture that can support this simulation before introducing real underwater hardware, acoustic modem drivers, or advanced AI.