#!/bin/bash
# Local test wrapper: the step itself is main.go (see step.yml, toolkit go).
set -e

THIS_SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
export BITRISE_STEP_SOURCE_DIR="${BITRISE_STEP_SOURCE_DIR:-$THIS_SCRIPT_DIR}"

cd "$THIS_SCRIPT_DIR"
exec go run .
