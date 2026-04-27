#!/usr/bin/env bash
set -euo pipefail

PROJECT_DIR="${1:?Usage: validate-project-structure.sh <project-directory>}"
ERRORS=0

red()   { printf "\033[0;31mFAIL: %s\033[0m\n" "$1"; }
green() { printf "\033[0;32mPASS: %s\033[0m\n" "$1"; }

check_file() {
    if [[ -f "${PROJECT_DIR}/$1" ]]; then
        green "File exists: $1"
    else
        red "Missing file: $1"
        ERRORS=$((ERRORS + 1))
    fi
}

check_dir() {
    if [[ -d "${PROJECT_DIR}/$1" ]]; then
        green "Directory exists: $1"
    else
        red "Missing directory: $1"
        ERRORS=$((ERRORS + 1))
    fi
}

check_makefile_target() {
    if grep -qE "^$1:" "${PROJECT_DIR}/Makefile" 2>/dev/null; then
        green "Makefile target: $1"
    else
        red "Missing Makefile target: $1"
        ERRORS=$((ERRORS + 1))
    fi
}

check_project_field() {
    if grep -q "$1" "${PROJECT_DIR}/PROJECT" 2>/dev/null; then
        green "PROJECT field: $1"
    else
        red "Missing PROJECT field: $1"
        ERRORS=$((ERRORS + 1))
    fi
}

echo "=== Validating Operator Project Structure ==="
echo "Project directory: ${PROJECT_DIR}"
echo ""

echo "--- Required Files ---"
check_file "PROJECT"
check_file "Makefile"
check_file "Dockerfile"
check_file "go.mod"
check_file "cmd/main.go"
check_file ".gitignore"
check_file ".dockerignore"
check_file ".golangci.yml"
check_file "README.md"
check_file "hack/boilerplate.go.txt"

echo ""
echo "--- Required Directories ---"
check_dir "api"
check_dir "internal/controller"
check_dir "config/default"
check_dir "config/manager"
check_dir "config/rbac"
check_dir "config/crd"
check_dir "config/samples"
check_dir "config/prometheus"
check_dir "config/scorecard"
check_dir "config/manifests"

echo ""
echo "--- Config Files (P1 — required for make bundle) ---"
check_file "config/manifests/kustomization.yaml"
check_file "config/scorecard/bases/config.yaml"
check_file "config/scorecard/kustomization.yaml"
check_file "config/scorecard/patches/basic.config.yaml"
check_file "config/scorecard/patches/olm.config.yaml"
check_file "config/crd/kustomizeconfig.yaml"

echo ""
echo "--- Config Files (P2 — production features) ---"
check_file "config/default/manager_auth_proxy_patch.yaml"
check_file "config/default/manager_config_patch.yaml"
check_file "config/rbac/auth_proxy_client_clusterrole.yaml"
check_file "config/rbac/auth_proxy_role.yaml"
check_file "config/rbac/auth_proxy_role_binding.yaml"
check_file "config/rbac/auth_proxy_service.yaml"
check_file "config/prometheus/kustomization.yaml"
check_file "config/prometheus/monitor.yaml"

echo ""
echo "--- Makefile Targets ---"
check_makefile_target "generate"
check_makefile_target "manifests"
check_makefile_target "test"
check_makefile_target "build"
check_makefile_target "docker-build"
check_makefile_target "docker-push"
check_makefile_target "deploy"
check_makefile_target "undeploy"
check_makefile_target "install"
check_makefile_target "uninstall"
check_makefile_target "bundle"

echo ""
echo "--- PROJECT File Fields ---"
check_project_field "domain:"
check_project_field "projectName:"
check_project_field "repo:"
check_project_field "layout:"
check_project_field "resources:"
check_project_field "kind:"
check_project_field "version:"

echo ""
echo "=== Results ==="
if [[ ${ERRORS} -eq 0 ]]; then
    green "All checks passed!"
    exit 0
else
    red "${ERRORS} check(s) failed"
    exit 1
fi
