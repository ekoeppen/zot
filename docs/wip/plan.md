# Implementation Plan

## Scope

## Goal

### Objective

Add before_model_request interceptor hook to zot extensions for dynamic message views without mutating session ground truth, and update documentation.

### Success criteria

All unit and integration tests pass, go vet/gofmt pass, and docs/extensions.md is updated with the new hook and examples.

## Design

### Decisions

### Packages

### Functions

### Structures

## Steps

### [x] Provider Message Unmarshaling

#### Precondition

Existing codebase is clean.

#### Action

Add UnmarshalMessage and UnmarshalMessages helpers in packages/provider/provider.go or packages/provider/messages.go, and refactor core/session.go to use it.

#### Result

Provider messages can be cleanly unmarshaled from JSON across packages.

#### Verification

Run provider and core tests.

### [x] Core Agent Hook

#### Precondition

Provider message unmarshaling is in place.

#### Action

Add BeforeModelRequest hook to core.Agent and wire it in packages/core/agent.go. Ensure repairToolUseResultPairs runs on the output.

#### Result

core.Agent supports dynamically transforming messages before sending provider requests.

#### Verification

Add unit test in packages/core for BeforeModelRequest and verify with go test ./packages/core.

### [x] Extension Protocol Support

#### Precondition

Core Agent hook is ready.

#### Action

Update extproto with before_model_request support in EventInterceptFromHost and EventInterceptResponseFromExt.

#### Result

extproto supports before_model_request interception.

#### Verification

Run go test ./packages/agent/extproto.

### [x] Extensions Manager & CLI Wiring

#### Precondition

Protocol support is in place.

#### Action

Implement InterceptBeforeModelRequest in packages/agent/extensions and wire it in packages/agent/cli.go (and agent wiring).

#### Result

Extensions manager handles before_model_request interceptors and wires them into Agent.

#### Verification

Run extensions unit tests and agent tests.

### [x] Go SDK & Test Suite

#### Precondition

Extensions manager is wired.

#### Action

Add InterceptBeforeModelRequest in packages/agent/ext (Go SDK) and write comprehensive unit tests across packages.

#### Result

Go SDK exposes InterceptBeforeModelRequest hook and all tests pass.

#### Verification

Run go test ./packages/agent/ext and all package tests.

### [x] Documentation Update

#### Precondition

Code and tests are implemented and passing.

#### Action

Update docs/extensions.md with before_model_request interceptor details, schema, payload examples, and SDK usage.

#### Result

docs/extensions.md contains complete documentation for before_model_request.

#### Verification

Inspect docs/extensions.md and git diff.

