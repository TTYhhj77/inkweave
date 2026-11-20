package app.inkweave.studio

import android.content.Context
import android.net.ConnectivityManager
import android.util.Base64
import org.json.JSONObject
import java.io.File
import java.security.KeyStore
import javax.net.ssl.TrustManagerFactory
import javax.net.ssl.X509TrustManager

data class LocalEngineInfo(
    val port: Int,
    val token: String,
    val baseUrl: String,
)

/**
 * Manages the embedded pure-Go Inkweave engine bundled inside nativeLibraryDir/libinkweave.so.
 * Exports Android's system CA bundle and active DNS servers so HTTPS calls to AI APIs, GitHub,
 * and Google Drive work seamlessly on Android 8–15 without CGO or NDK.
 */
class GoEngineProcess(private val context: Context) {

    @Volatile
    private var proc: Process? = null

    @Volatile
    var info: LocalEngineInfo? = null
        private set

    fun start(): LocalEngineInfo {
        info?.let { existing ->
            if (proc?.isAlive == true) return existing
        }

        val binFile = File(context.applicationInfo.nativeLibraryDir, "libinkweave.so")
        if (!binFile.exists()) {
            throw IllegalStateException("Inkweave engine binary not found at ${binFile.absolutePath}")
        }

        val readyFile = File(context.cacheDir, "engine-ready.json")
        if (readyFile.exists()) readyFile.delete()

        val caFile = exportAndroidCaBundle()
        val dnsList = resolveAndroidDnsServers()

        val pb = ProcessBuilder(
            binFile.absolutePath,
            "-host", "127.0.0.1",
            "-port", "0",
            "-no-open",
            "-data-dir", context.filesDir.absolutePath,
            "-ready-file", readyFile.absolutePath,
        )
        pb.directory(context.filesDir)
        pb.redirectErrorStream(true)

        val env = pb.environment()
        env["HOME"] = context.filesDir.absolutePath
        env["TMPDIR"] = context.cacheDir.absolutePath
        env["SSL_CERT_FILE"] = caFile.absolutePath
        env["INKWEAVE_DNS_SERVERS"] = dnsList

        val started = pb.start()
        proc = started

        // Drain stdout in background so buffer never blocks
        Thread {
            runCatching {
                started.inputStream.bufferedReader().use { r ->
                    while (r.readLine() != null) { /* discard */ }
                }
            }
        }.apply { isDaemon = true }.start()

        val deadline = System.currentTimeMillis() + 15_000L
        while (System.currentTimeMillis() < deadline) {
            if (readyFile.exists() && readyFile.length() > 10) {
                val parsed = runCatching {
                    val obj = JSONObject(readyFile.readText())
                    LocalEngineInfo(
                        port = obj.getInt("port"),
                        token = obj.getString("token"),
                        baseUrl = obj.getString("base_url"),
                    )
                }.getOrNull()
                if (parsed != null) {
                    info = parsed
                    return parsed
                }
            }
            if (!started.isAlive) {
                throw IllegalStateException("Inkweave engine exited unexpectedly with code ${started.exitValue()}")
            }
            Thread.sleep(60)
        }
        stop()
        throw IllegalStateException("Timed out waiting for Inkweave engine to start")
    }

    fun stop() {
        val p = proc
        proc = null
        info = null
        if (p != null) {
            runCatching { p.destroy() }
        }
    }

    private fun exportAndroidCaBundle(): File {
        val out = File(context.filesDir, "android-ca-bundle.pem")
        if (out.exists() && out.length() > 10_000) return out
        val sb = StringBuilder()
        runCatching {
            val tmf = TrustManagerFactory.getInstance(TrustManagerFactory.getDefaultAlgorithm())
            tmf.init(null as KeyStore?)
            for (tm in tmf.trustManagers) {
                if (tm is X509TrustManager) {
                    for (cert in tm.acceptedIssuers) {
                        val b64 = Base64.encodeToString(cert.encoded, Base64.NO_WRAP)
                        sb.append("-----BEGIN CERTIFICATE-----\n")
                        var idx = 0
                        while (idx < b64.length) {
                            val end = minOf(idx + 64, b64.length)
                            sb.append(b64, idx, end).append('\n')
                            idx = end
                        }
                        sb.append("-----END CERTIFICATE-----\n")
                    }
                }
            }
        }
        if (sb.isNotEmpty()) {
            out.writeText(sb.toString())
        }
        return out
    }

    private fun resolveAndroidDnsServers(): String {
        val list = LinkedHashSet<String>()
        runCatching {
            val cm = context.getSystemService(Context.CONNECTIVITY_SERVICE) as? ConnectivityManager
            val network = cm?.activeNetwork
            val props = if (network != null) cm.getLinkProperties(network) else null
            props?.dnsServers?.forEach { addr ->
                val host = addr.hostAddress?.substringBefore('%')
                if (!host.isNullOrBlank()) list.add(host)
            }
        }
        list.add("1.1.1.1")
        list.add("8.8.8.8")
        return list.joinToString(",")
    }
}
