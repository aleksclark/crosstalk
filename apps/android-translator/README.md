# CrossTalk Android Translator

Native Kotlin / Jetpack Compose translator client for CrossTalk.

This module is hermetic: it carries its own Gradle wrapper and pinned toolchain.
The server OpenAPI document at `api/openapi.json` is the contract authority —
Android never rewrites it.

## Prerequisites

| Tool | Pin / path |
|---|---|
| JDK | 17 (`JAVA_HOME=/usr/lib/jvm/java-17-openjdk`) |
| Android SDK | `ANDROID_HOME=/opt/android-sdk` with platform **35** |
| Gradle | Wrapper **8.11.1** (no global Gradle required) |
| AGP | **8.9.2** |
| Kotlin / Compose compiler | **2.1.20** |
| compileSdk / targetSdk | **35** |
| minSdk | **26** |
| applicationId | `com.crosstalk.translator` |

Create a gitignored `local.properties` (or let tasks create it):

```properties
sdk.dir=/opt/android-sdk
```

## Package layout

```
app/src/main/java/com/crosstalk/translator/
  CrossTalkApplication.kt
  MainActivity.kt
  app/AppContainer.kt
  auth/          # Phase 2+
  contract/      # domain models + CrossTalkApi (+ GeneratedApiAdapter only)
  network/       # OkHttp factory, TLS, redacting listener
  rtc/           # Phase 3+
  service/       # Phase 4+
  audio/         # Phase 4+
  feature/       # Phase 2/5 UI routes
  ui/            # Phase 5 design system
  util/          # Clock, SecretRedactor
```

Generated OpenAPI sources land in `app/build/generated/openapi` under:

- `com.crosstalk.translator.generated.api`
- `com.crosstalk.translator.generated.model`

Only `contract/` may import generated packages.

## Common tasks

From this directory:

```bash
export JAVA_HOME=/usr/lib/jvm/java-17-openjdk
export ANDROID_HOME=/opt/android-sdk
export ANDROID_SDK_ROOT=/opt/android-sdk

./gradlew --no-daemon :app:assembleDebug
./gradlew --no-daemon :app:assembleRelease
./gradlew --no-daemon :app:lintDebug
./gradlew --no-daemon :app:testDebugUnitTest
./gradlew --no-daemon openApiGenerate
```

From the monorepo root:

```bash
task api:generate:android
task api:validate:android
```

## Build config

- Debug and release default base URL: `https://crosstalk-sfu.fly.dev`
- The login screen allows the operator to select another server URL; the
  normalized URL is persisted for session restore and future launches.
- A bare host is normalized to HTTPS. Release builds reject cleartext URLs;
  debug builds may use an explicit `http://` URL for local testing.

## Live-session tools

- **Show QR code** loads the session-scoped broadcast token and renders the
  public listener URL locally. Tokens are never written to logs or displayed as
  plaintext.
- **Route** expands the same core session-channel controls used by the web
  translator: list sources per channel, assign/remove sources, mute/unmute, and
  adjust mix level from 0–200%. Writes are serialized per channel and keep the
  last server-confirmed state when a save fails.
- QR encoding uses `com.google.zxing:core` only; it avoids sending the
  session-scoped broadcast URL to a third-party QR service.

## Pin notes

- Plan listed AndroidX test runner/core as `1.6.2`. Maven metadata shows
  `androidx.test:runner:1.6.2` exists, but **`androidx.test:core` has no 1.6.2**
  (latest 1.6.x is `1.6.1`). Catalog pins: core/rules `1.6.1`, runner `1.6.2`.

## Tests (Phase 6)

### Unit / static

```bash
./gradlew --no-daemon :app:testDebugUnitTest :app:lintDebug
./gradlew --no-daemon :app:assembleDebug :app:assembleRelease
task api:validate:android
```

### Instrumentation

```bash
# Lifecycle / Keystore / process-death (no server required)
./gradlew --no-daemon :app:connectedDebugAndroidTest

# Real assignment + Pion (requires live server + seed credentials)
./gradlew --no-daemon :app:connectedDebugAndroidTest \
  -Pandroid.testInstrumentationRunnerArguments.CROSSTALK_BASE_URL=http://10.0.2.2:8080 \
  -Pandroid.testInstrumentationRunnerArguments.CROSSTALK_TRANSLATOR_USER=... \
  -Pandroid.testInstrumentationRunnerArguments.CROSSTALK_TRANSLATOR_PASSWORD=...
```

Managed devices (optional, needs system images): `pixel2Api33`, `pixel2Api34`,
`pixel2Api35`. Prefer attached emulator/device via `connectedDebugAndroidTest`
when images are absent.

### Device golden (physical required for merge)

```bash
# From monorepo root — never echo passwords
export CROSSTALK_BASE_URL=http://<host>:8080
export CROSSTALK_ADMIN_PASSWORD='***'
bash test/android/run-device-golden.sh
```

- Physical path: capture label `physical-mic` (real mic + external tones).
- Emulator path: set `CROSSTALK_ALLOW_EMULATOR=1`; labelled
  `synthetic-capture-debug-only` and **not** merge proof.
- Process death uses `am kill` (not `force-stop`).

Evidence docs: [`docs/e2e/README.md`](docs/e2e/README.md),
[`docs/e2e/PHYSICAL_GOLDEN.md`](docs/e2e/PHYSICAL_GOLDEN.md).

### Task aliases

```bash
task test:android:unit
task test:android:connected
task test:android:golden
```
