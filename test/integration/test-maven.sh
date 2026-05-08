#!/usr/bin/env bash
# Verify the Maven launcher + host-platform classifier JAR install and exec
# the bundled binary correctly.
#
# Self-contained: builds its own binary stamped with a Maven-friendly test
# version, copies it into the host platform's resource path, runs `mvn install`
# into a scratch local repository, then executes
# `java -cp <launcher>:<classifier> Main --version` and confirms the output
# matches the stamped version.
#
# Cleans up by reverting pom versions and removing scratch state on exit.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
# Maven accepts hyphenated qualifiers like `0.0.0-test` (treated as a
# pre-release). We avoid `-SNAPSHOT` so the stamped poms don't trip OSSRH
# snapshot/release routing in case anyone misuses this script.
TEST_VERSION="0.0.0-test"
TEST_BIN_DIR="$REPO_ROOT/test/.scratch/maven-dist"
SCRATCH="$REPO_ROOT/test/.scratch/maven"
LOCAL_REPO="$SCRATCH/m2"

case "$(uname -s)" in
    MINGW*|MSYS*|CYGWIN*) BIN_NAME="sqlitedeploy.exe" ;;
    *)                    BIN_NAME="sqlitedeploy" ;;
esac

mkdir -p "$TEST_BIN_DIR"
echo "--- building test binary (version=$TEST_VERSION)"
go build -ldflags="-X main.version=$TEST_VERSION" \
    -o "$TEST_BIN_DIR/$BIN_NAME" \
    ./cmd/sqlitedeploy

export TEST_BIN_DIR
source "$REPO_ROOT/test/lib/platform.sh"

# Map host go-os/arch to its Maven classifier name.
case "$HOST_OS-$HOST_GOARCH" in
    linux-amd64)   MVN_CLASSIFIER=linux-x86_64 ;;
    linux-arm64)   MVN_CLASSIFIER=linux-aarch64 ;;
    darwin-amd64)  MVN_CLASSIFIER=darwin-x86_64 ;;
    darwin-arm64)  MVN_CLASSIFIER=darwin-aarch64 ;;
    windows-amd64) MVN_CLASSIFIER=windows-x86_64 ;;
    windows-arm64) MVN_CLASSIFIER=windows-aarch64 ;;
    *) echo "unsupported host: $HOST_OS-$HOST_GOARCH" >&2; exit 1 ;;
esac

cleanup() {
    rc=$?
    echo "--- maven test cleanup"
    node "$REPO_ROOT/scripts/stamp-versions-maven.js" 0.0.0 >/dev/null || true
    rm -rf "$SCRATCH" "$TEST_BIN_DIR"
    rm -rf "$REPO_ROOT/packaging/maven/launcher/target"
    rm -rf "$REPO_ROOT/packaging/maven/platforms/$MVN_CLASSIFIER/target"
    rm -rf "$REPO_ROOT/packaging/maven/platforms/$MVN_CLASSIFIER/src/main/resources/META-INF/native"
    if [ "$rc" -eq 0 ]; then echo "PASS  test-maven"; else echo "FAIL  test-maven (rc=$rc)"; fi
    exit "$rc"
}
trap cleanup EXIT

if ! command -v mvn >/dev/null 2>&1; then
    echo "mvn not found — install Maven (https://maven.apache.org/install.html) to run this test" >&2
    exit 1
fi
if ! command -v java >/dev/null 2>&1; then
    echo "java not found — install a JDK (Java 8+) to run this test" >&2
    exit 1
fi

echo "--- maven test (host=$MVN_CLASSIFIER, version=$TEST_VERSION)"
echo "    binary: $HOST_BIN_PATH"

mkdir -p "$SCRATCH" "$LOCAL_REPO"

echo "[1/4] stamp test version into pom.xml files"
node "$REPO_ROOT/scripts/stamp-versions-maven.js" "$TEST_VERSION" >/dev/null

echo "[2/4] copy host binary into platform module resource dir"
DST_DIR="$REPO_ROOT/packaging/maven/platforms/$MVN_CLASSIFIER/src/main/resources/META-INF/native"
mkdir -p "$DST_DIR"
cp "$HOST_BIN_PATH" "$DST_DIR/$BIN_NAME"
chmod +x "$DST_DIR/$BIN_NAME" || true

echo "[3/4] mvn install launcher + host classifier into scratch local repo"
# Build only the modules we need: -pl picks them, -am pulls the parent pom.
(cd "$REPO_ROOT/packaging/maven" && \
    mvn -B -Dmaven.repo.local="$LOCAL_REPO" \
        -pl launcher,platforms/$MVN_CLASSIFIER -am \
        install)

LAUNCHER_JAR="$LOCAL_REPO/io/github/khangdang1690/sqlitedeploy-cli/$TEST_VERSION/sqlitedeploy-cli-$TEST_VERSION.jar"
PLATFORM_JAR="$LOCAL_REPO/io/github/khangdang1690/sqlitedeploy-cli-$MVN_CLASSIFIER/$TEST_VERSION/sqlitedeploy-cli-$MVN_CLASSIFIER-$TEST_VERSION.jar"

if [ ! -f "$LAUNCHER_JAR" ]; then
    echo "    launcher jar not found at $LAUNCHER_JAR" >&2
    exit 1
fi
if [ ! -f "$PLATFORM_JAR" ]; then
    echo "    platform jar not found at $PLATFORM_JAR" >&2
    exit 1
fi

# Build a classpath string Java's `java.exe` will accept. On Windows we use
# `;` between entries and convert each path to native form via `cygpath -w`,
# otherwise Git Bash's MSYS layer doesn't translate paths inside a multi-
# entry argument and Java interprets the mixed string incorrectly.
if [ "$HOST_OS" = "windows" ]; then
    LAUNCHER_JAR_CP="$(cygpath -w "$LAUNCHER_JAR")"
    PLATFORM_JAR_CP="$(cygpath -w "$PLATFORM_JAR")"
    CLASSPATH="$LAUNCHER_JAR_CP;$PLATFORM_JAR_CP"
else
    CLASSPATH="$LAUNCHER_JAR:$PLATFORM_JAR"
fi

echo "[4/4] exec launcher --version via java -cp"
ACTUAL=$(java -cp "$CLASSPATH" io.github.khangdang1690.sqlitedeploy.Main --version)
EXPECTED="sqlitedeploy version $TEST_VERSION"
if [ "$ACTUAL" != "$EXPECTED" ]; then
    echo "    expected: $EXPECTED"
    echo "    got:      $ACTUAL"
    exit 1
fi
echo "    output matches: $ACTUAL"

echo "    sanity check --help (verifies stdio piping)"
java -cp "$CLASSPATH" io.github.khangdang1690.sqlitedeploy.Main --help >/dev/null
echo "    --help exits 0"
