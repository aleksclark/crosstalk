#!/usr/bin/env bash
# Validate OpenAPI generation for the Android translator client.
# Regenerates into a clean directory and fails if generation or compile breaks.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
APP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SPEC="${ROOT_DIR}/api/openapi.json"
JAVA_HOME="${JAVA_HOME:-/usr/lib/jvm/java-17-openjdk}"
ANDROID_HOME="${ANDROID_HOME:-/opt/android-sdk}"
ANDROID_SDK_ROOT="${ANDROID_SDK_ROOT:-$ANDROID_HOME}"
export JAVA_HOME ANDROID_HOME ANDROID_SDK_ROOT
export PATH="${JAVA_HOME}/bin:${PATH}"

if [[ ! -f "${SPEC}" ]]; then
  echo "error: missing OpenAPI spec at ${SPEC}" >&2
  exit 1
fi

if [[ ! -d "${ANDROID_HOME}/platforms/android-35" ]]; then
  echo "error: Android SDK platform 35 not installed under ${ANDROID_HOME}" >&2
  exit 1
fi

if [[ ! -x "${JAVA_HOME}/bin/java" ]]; then
  echo "error: JDK 17 not found at ${JAVA_HOME}" >&2
  exit 1
fi

java_ver="$("${JAVA_HOME}/bin/java" -version 2>&1 | head -n1 || true)"
echo "Using ${java_ver}"
echo "Using ANDROID_HOME=${ANDROID_HOME}"
echo "Spec: ${SPEC}"

# Ensure local.properties exists for AGP (gitignored).
if [[ ! -f "${APP_DIR}/local.properties" ]]; then
  printf 'sdk.dir=%s\n' "${ANDROID_HOME}" > "${APP_DIR}/local.properties"
fi

cd "${APP_DIR}"

# Clean previous generated OpenAPI output then regenerate + compile unit tests
# that assert contract paths and import boundaries.
./gradlew --no-daemon cleanOpenApiGenerate openApiGenerate \
  :app:compileDebugUnitTestKotlin \
  :app:testDebugUnitTest \
  --tests 'com.crosstalk.translator.OpenApiContractTest' \
  --tests 'com.crosstalk.translator.GeneratedImportBoundaryTest'

echo "api:validate:android OK"
