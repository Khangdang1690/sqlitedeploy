# sqlitedeploy — Maven distribution

Maven Central artifacts that ship the prebuilt `sqlitedeploy` CLI binary as a
classifier-style native dependency, mirroring the npm
(`@weirdvl/<platform>`) and pip (`platform-tagged wheels`) layouts elsewhere
in this repo.

## Coordinates

* `io.github.khangdang1690:sqlitedeploy-cli` — pure-Java launcher that detects
  the host OS/arch, locates the matching native binary on the classpath,
  extracts it to the user cache directory, and execs it with the user's
  arguments.
* `io.github.khangdang1690:sqlitedeploy-cli-<os>-<arch>` — JAR containing the
  native binary at `META-INF/native/sqlitedeploy[.exe]`. Six classifiers are
  published, one per supported platform:
    - `linux-x86_64`, `linux-aarch64`
    - `darwin-x86_64`, `darwin-aarch64`
    - `windows-x86_64`, `windows-aarch64`

## Usage

Add the launcher and one matching platform artifact to your project:

```xml
<dependency>
  <groupId>io.github.khangdang1690</groupId>
  <artifactId>sqlitedeploy-cli</artifactId>
  <version>${sqlitedeploy.version}</version>
</dependency>
<dependency>
  <groupId>io.github.khangdang1690</groupId>
  <artifactId>sqlitedeploy-cli-linux-x86_64</artifactId>
  <version>${sqlitedeploy.version}</version>
  <scope>runtime</scope>
</dependency>
```

In CI/CD or polyglot teams, prefer
[`os-maven-plugin`](https://github.com/trustin/os-maven-plugin) to populate
`${os.detected.classifier}` automatically:

```xml
<dependency>
  <groupId>io.github.khangdang1690</groupId>
  <artifactId>sqlitedeploy-cli-${os.detected.classifier}</artifactId>
  <version>${sqlitedeploy.version}</version>
  <scope>runtime</scope>
</dependency>
```

Run via `mvn`:

```bash
mvn exec:java -Dexec.mainClass=io.github.khangdang1690.sqlitedeploy.Main -Dexec.args="--help"
```

Or invoke `Main.main(...)` from your own code.

## How it resolves the binary

The launcher (`io.github.khangdang1690.sqlitedeploy.Main`) maps `os.name` and
`os.arch` to one of the six classifier names, then looks for
`/META-INF/native/sqlitedeploy[.exe]` on the classpath. Each classifier JAR
ships exactly that resource for its target platform; pulling in two
classifier JARs at once is unnecessary and would result in whichever JAR
loads first winning the resource lookup.

The binary is extracted once per launcher version into
`~/.cache/sqlitedeploy/<version>/sqlitedeploy[.exe]`, made executable on
POSIX, and execed via `ProcessBuilder.inheritIO()` so prompts (e.g. during
`auth login`) and replication logs flow through unchanged.

## Building locally

The platform classifier JARs are *not* committed with binaries; the build
populates each module's `src/main/resources/META-INF/native/` directory at
release time from the cross-compiled binaries in `dist/`.

```bash
make release VERSION=0.1.0       # cross-compile all six binaries into dist/
make package-maven VERSION=0.1.0 # copy binaries into modules + run mvn package
```

The output JARs land under `packaging/maven/launcher/target/` and
`packaging/maven/platforms/*/target/`.

## Publishing

Maven Central via the Sonatype **Central Portal**
(https://central.sonatype.com), the modern replacement for the legacy
OSSRH pipeline. The `release` profile attaches sources/javadoc JARs, signs
every artifact with GPG, and uses `central-publishing-maven-plugin` to
upload + auto-publish to Maven Central. CI runs `mvn -B -P release deploy`;
see [`.github/workflows/release.yml`](../../.github/workflows/release.yml).

One-time prerequisites for the first publish:

1. Sign in at https://central.sonatype.com/account with GitHub
   (Khangdang1690), claim the `io.github.khangdang1690` namespace
   (auto-verified because the prefix matches the GitHub username), and
   generate a publishing user token.
2. Generate a 4096-bit RSA GPG signing key and publish the public part to
   `keys.openpgp.org` and `keyserver.ubuntu.com`.
3. Set four GitHub Actions repo secrets:
   - `CENTRAL_USERNAME` and `CENTRAL_PASSWORD` from the Central Portal
     user token,
   - `MAVEN_GPG_PRIVATE_KEY` (ASCII-armored secret key) and
     `MAVEN_GPG_PASSPHRASE`.
