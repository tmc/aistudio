# HTTP Record and Replay (httprr)

This package was imported from github.com/golang/oscar-internal/internal/httprr (commit 3aee0b9db89d66cf4e6396176f7238eea8def8e2) using git-subtree.

## Attribution

Copyright 2024 The Go Authors. All rights reserved.
Use of this source code is governed by a BSD-style license.

## Description

Package httprr implements HTTP record and replay, mainly for use in tests. It allows HTTP interactions to be recorded once and then replayed in subsequent test runs, making tests more deterministic and faster.