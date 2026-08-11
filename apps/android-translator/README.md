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

- Debug default base URL: `http://10.0.2.2:8080` (emulator loopback to host)
- Release default base URL: `https://crosstalk.local` (override via Gradle property)
- Release cleartext traffic is disabled; debug may allow localhost via network security config override.

## Pin notes

- Plan listed AndroidX test runner/core as `1.6.2`. Maven metadata shows
  `androidx.test:runner:1.6.2` exists, but **`androidx.test:core` has no 1.6.2**
  (latest 1.6.x is `1.6.1`). Catalog pins: core/rules `1.6.1`, runner `1.6.2`.
