package io.github.khangdang1690.sqlitedeploy;

import java.io.IOException;
import java.io.InputStream;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.nio.file.StandardCopyOption;
import java.nio.file.attribute.PosixFilePermission;
import java.util.ArrayList;
import java.util.EnumSet;
import java.util.List;
import java.util.Locale;
import java.util.Set;

/**
 * Resolver shim that locates the platform-specific sqlitedeploy binary
 * embedded inside one of the {@code sqlitedeploy-cli-<os>-<arch>} classifier
 * JARs on the classpath, extracts it to the user cache dir, and execs it
 * with the user's args.
 *
 * <p>Mirrors the npm shim ({@code packaging/npm/sqlitedeploy/bin/sqlitedeploy.js})
 * and pip resolver ({@code packaging/pip/src/sqlitedeploy/_resolve.py}).
 */
public final class Main {

    private static final String GITHUB_RELEASES =
        "https://github.com/Khangdang1690/sqlitedeploy/releases";

    private Main() {}

    public static void main(String[] args) throws IOException, InterruptedException {
        String classifier = detectClassifier();
        boolean windows = classifier.startsWith("windows-");
        String binaryName = windows ? "sqlitedeploy.exe" : "sqlitedeploy";
        String resourcePath = "/META-INF/native/" + binaryName;

        Path extracted = extractBinary(resourcePath, binaryName, windows, classifier);

        List<String> command = new ArrayList<>(args.length + 1);
        command.add(extracted.toAbsolutePath().toString());
        for (String arg : args) {
            command.add(arg);
        }
        ProcessBuilder pb = new ProcessBuilder(command).inheritIO();
        Process proc = pb.start();
        System.exit(proc.waitFor());
    }

    private static String detectClassifier() {
        String os = System.getProperty("os.name", "").toLowerCase(Locale.ROOT);
        String arch = System.getProperty("os.arch", "").toLowerCase(Locale.ROOT);

        String osTag;
        if (os.contains("linux")) {
            osTag = "linux";
        } else if (os.contains("mac") || os.contains("darwin")) {
            osTag = "darwin";
        } else if (os.contains("win")) {
            osTag = "windows";
        } else {
            throw new UnsupportedOperationException(
                "sqlitedeploy: unsupported OS '" + os + "'. " +
                "Supported: linux, macOS, Windows. " +
                "Download a binary manually from " + GITHUB_RELEASES);
        }

        String archTag;
        if (arch.equals("amd64") || arch.equals("x86_64") || arch.equals("x64")) {
            archTag = "x86_64";
        } else if (arch.equals("aarch64") || arch.equals("arm64")) {
            archTag = "aarch64";
        } else {
            throw new UnsupportedOperationException(
                "sqlitedeploy: unsupported arch '" + arch + "'. " +
                "Supported: x86_64, aarch64. " +
                "Download a binary manually from " + GITHUB_RELEASES);
        }

        return osTag + "-" + archTag;
    }

    private static Path extractBinary(String resourcePath, String binaryName,
                                      boolean windows, String classifier)
            throws IOException {
        Path cacheDir = userCacheDir().resolve("sqlitedeploy").resolve(pkgVersion());
        Files.createDirectories(cacheDir);
        Path dst = cacheDir.resolve(binaryName);

        try (InputStream in = Main.class.getResourceAsStream(resourcePath)) {
            if (in == null) {
                throw new IllegalStateException(
                    "sqlitedeploy: platform classifier JAR for " + classifier +
                    " is not on the classpath. Add a runtime dependency on " +
                    "io.github.khangdang1690:sqlitedeploy-cli-" + classifier +
                    " (or use os-maven-plugin to auto-detect the classifier). " +
                    "Manual download: " + GITHUB_RELEASES);
            }
            // Always overwrite: cheap (~40 MB) and avoids stale cache after
            // version bumps. The cache dir is itself version-scoped above.
            Files.copy(in, dst, StandardCopyOption.REPLACE_EXISTING);
        }

        if (!windows) {
            try {
                Set<PosixFilePermission> perms = EnumSet.of(
                    PosixFilePermission.OWNER_READ,
                    PosixFilePermission.OWNER_WRITE,
                    PosixFilePermission.OWNER_EXECUTE,
                    PosixFilePermission.GROUP_READ,
                    PosixFilePermission.GROUP_EXECUTE,
                    PosixFilePermission.OTHERS_READ,
                    PosixFilePermission.OTHERS_EXECUTE
                );
                Files.setPosixFilePermissions(dst, perms);
            } catch (UnsupportedOperationException ignore) {
                dst.toFile().setExecutable(true);
            }
        }
        return dst;
    }

    private static Path userCacheDir() {
        String home = System.getProperty("user.home", "");
        return Paths.get(home, ".cache");
    }

    private static String pkgVersion() {
        Package p = Main.class.getPackage();
        String v = (p != null) ? p.getImplementationVersion() : null;
        return (v != null && !v.isEmpty()) ? v : "dev";
    }
}
