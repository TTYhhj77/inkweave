package app.inkweave.studio

import android.annotation.SuppressLint
import android.content.Intent
import android.graphics.Color
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.provider.DocumentsContract
import android.view.ViewGroup
import android.webkit.DownloadListener
import android.webkit.JavascriptInterface
import android.webkit.JsResult
import android.webkit.WebChromeClient
import android.webkit.WebResourceRequest
import android.webkit.WebSettings
import android.webkit.WebView
import android.webkit.WebViewClient
import android.widget.FrameLayout
import android.widget.Toast
import androidx.activity.OnBackPressedCallback
import androidx.activity.result.contract.ActivityResultContracts
import androidx.appcompat.app.AlertDialog
import androidx.appcompat.app.AppCompatActivity
import androidx.core.content.FileProvider
import androidx.core.view.ViewCompat
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsCompat
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import org.json.JSONObject
import java.io.File
import java.net.HttpURLConnection
import java.net.URL

class MainActivity : AppCompatActivity() {

    private lateinit var rootContainer: FrameLayout
    private lateinit var webView: WebView
    private lateinit var engine: GoEngineProcess
    private lateinit var safSync: SafNovelSync

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main)
    private var syncLoopJob: Job? = null
    private var engineInfo: LocalEngineInfo? = null
    private var pageLoaded = false

    private val folderPickerLauncher = registerForActivityResult(
        ActivityResultContracts.OpenDocumentTree()
    ) { treeUri: Uri? ->
        if (treeUri == null) return@registerForActivityResult
        scope.launch {
            Toast.makeText(
                this@MainActivity,
                "กำลังเชื่อมต่อและซิงก์โฟลเดอร์นิยาย...",
                Toast.LENGTH_SHORT
            ).show()
            val status = withContext(Dispatchers.IO) {
                safSync.attachFolder(treeUri)
            }
            notifyWebFolderChanged(
                status,
                "เชื่อมต่อโฟลเดอร์ ${status.label} เรียบร้อยแล้ว (อ่าน/เขียน/แก้ไข/ลบอัตโนมัติ)"
            )
            startSyncLoop()
        }
    }

    @SuppressLint("SetJavaScriptEnabled")
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        engine = GoEngineProcess(applicationContext)
        safSync = SafNovelSync(applicationContext)

        // Consistent edge-to-edge across Android 8–15 with native inset padding
        // so the top status bar / camera cutout and bottom nav bar / keyboard NEVER overlap UI.
        WindowCompat.setDecorFitsSystemWindows(window, false)

        rootContainer = FrameLayout(this).apply {
            layoutParams = ViewGroup.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                ViewGroup.LayoutParams.MATCH_PARENT
            )
        }
        webView = WebView(this).apply {
            layoutParams = FrameLayout.LayoutParams(
                FrameLayout.LayoutParams.MATCH_PARENT,
                FrameLayout.LayoutParams.MATCH_PARENT
            )
        }
        rootContainer.addView(webView)
        setContentView(rootContainer)

        ViewCompat.setOnApplyWindowInsetsListener(rootContainer) { v, insets ->
            val safe = insets.getInsets(
                WindowInsetsCompat.Type.systemBars() or
                    WindowInsetsCompat.Type.displayCutout() or
                    WindowInsetsCompat.Type.ime()
            )
            v.setPadding(safe.left, safe.top, safe.right, safe.bottom)
            WindowInsetsCompat.CONSUMED
        }
        ViewCompat.requestApplyInsets(rootContainer)

        applyNativeTheme("paper")

        with(webView.settings) {
            javaScriptEnabled = true
            domStorageEnabled = true
            databaseEnabled = true
            allowFileAccess = false
            allowContentAccess = true
            mixedContentMode = WebSettings.MIXED_CONTENT_NEVER_ALLOW
            useWideViewPort = true
            loadWithOverviewMode = true
        }

        webView.addJavascriptInterface(NativeBridge(), "InkweaveNative")

        webView.webChromeClient = object : WebChromeClient() {
            override fun onJsConfirm(
                view: WebView?,
                url: String?,
                message: String?,
                result: JsResult?
            ): Boolean {
                AlertDialog.Builder(this@MainActivity)
                    .setMessage(message ?: "")
                    .setPositiveButton("ตกลง") { _, _ -> result?.confirm() }
                    .setNegativeButton("ยกเลิก") { _, _ -> result?.cancel() }
                    .setOnCancelListener { result?.cancel() }
                    .show()
                return true
            }

            override fun onJsAlert(
                view: WebView?,
                url: String?,
                message: String?,
                result: JsResult?
            ): Boolean {
                AlertDialog.Builder(this@MainActivity)
                    .setMessage(message ?: "")
                    .setPositiveButton("ตกลง") { _, _ -> result?.confirm() }
                    .setOnCancelListener { result?.confirm() }
                    .show()
                return true
            }
        }

        webView.webViewClient = object : WebViewClient() {
            override fun onPageFinished(view: WebView?, url: String?) {
                super.onPageFinished(view, url)
                pageLoaded = true
                if (!safSync.wasPrompted() && safSync.getSavedTreeUri() == null) {
                    safSync.markPrompted()
                    showFirstLaunchFolderPrompt()
                }
            }

            override fun shouldOverrideUrlLoading(view: WebView?, request: WebResourceRequest?): Boolean {
                val uri = request?.url ?: return false
                val host = uri.host ?: ""
                if (host == "127.0.0.1" || host == "localhost") {
                    return false
                }
                startActivity(Intent(Intent.ACTION_VIEW, uri))
                return true
            }
        }

        webView.setDownloadListener(DownloadListener { url, _, contentDisposition, mimeType, _ ->
            scope.launch {
                downloadAndShare(url, contentDisposition, mimeType)
            }
        })

        onBackPressedDispatcher.addCallback(this, object : OnBackPressedCallback(true) {
            override fun handleOnBackPressed() {
                if (webView.canGoBack()) {
                    webView.goBack()
                } else {
                    isEnabled = false
                    onBackPressedDispatcher.onBackPressed()
                }
            }
        })

        scope.launch {
            try {
                // Pull any externally added/edited novels from dedicated Android folder before booting Go engine
                withContext(Dispatchers.IO) {
                    safSync.pullFromSaf()
                }
                val info = withContext(Dispatchers.IO) {
                    engine.start()
                }
                engineInfo = info
                webView.loadUrl(info.baseUrl)
                startSyncLoop()
            } catch (e: Exception) {
                AlertDialog.Builder(this@MainActivity)
                    .setTitle("ไม่สามารถเริ่มระบบ Inkweave ได้")
                    .setMessage(e.message ?: e.toString())
                    .setPositiveButton("ปิด") { _, _ -> finish() }
                    .show()
            }
        }
    }

    private fun showFirstLaunchFolderPrompt() {
        AlertDialog.Builder(this)
            .setTitle("สร้าง / เลือกโฟลเดอร์เก็บนิยาย")
            .setMessage(
                "คุณสามารถสร้างหรือเลือกโฟลเดอร์เฉพาะบน Android ให้ Inkweave ใช้เก็บนิยายได้โดยตรง\n\n" +
                    "• แอพจะมีสิทธิ์อ่าน เขียน แก้ไข และลบไฟล์เฉพาะในโฟลเดอร์ที่คุณเลือกเท่านั้น\n" +
                    "• สามารถเปิดดูไฟล์ Markdown/YAML ด้วยแอพจัดการไฟล์ หรือเก็บไว้ได้แม้ลบแอพ"
            )
            .setPositiveButton("สร้าง / เลือกโฟลเดอร์") { _, _ ->
                launchFolderPicker()
            }
            .setNegativeButton("ไว้ตั้งค่าทีหลัง", null)
            .show()
    }

    private fun launchFolderPicker() {
        val initialUri: Uri? = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            safSync.getSavedTreeUri()
                ?: DocumentsContract.buildDocumentUri(
                    "com.android.externalstorage.documents",
                    "primary:Documents"
                )
        } else {
            null
        }
        folderPickerLauncher.launch(initialUri)
    }

    private fun startSyncLoop() {
        syncLoopJob?.cancel()
        if (safSync.getSavedTreeUri() == null) return
        syncLoopJob = scope.launch(Dispatchers.IO) {
            while (isActive) {
                runCatching { safSync.syncLocalToSaf() }
                delay(4_000L)
            }
        }
    }

    private fun notifyWebFolderChanged(status: FolderStatus, message: String? = null) {
        if (!pageLoaded) return
        val jsonLiteral = JSONObject.quote(status.toJson(message))
        runOnUiThread {
            webView.evaluateJavascript(
                "window.__inkweaveOnAndroidFolderChanged && window.__inkweaveOnAndroidFolderChanged($jsonLiteral);",
                null
            )
        }
    }

    override fun onResume() {
        super.onResume()
        if (safSync.getSavedTreeUri() != null) {
            scope.launch {
                val pulled = withContext(Dispatchers.IO) {
                    safSync.pullFromSaf()
                }
                if (pulled) {
                    notifyWebFolderChanged(safSync.getStatus(), "อัปเดตไฟล์จากโฟลเดอร์ Android แล้ว")
                }
                startSyncLoop()
            }
        }
    }

    override fun onPause() {
        super.onPause()
        syncLoopJob?.cancel()
        if (safSync.getSavedTreeUri() != null) {
            Thread {
                runCatching { safSync.syncLocalToSaf() }
            }.start()
        }
    }

    private fun applyNativeTheme(theme: String) {
        val (bgHex, isLight) = when (theme) {
            "warm-dark" -> "#201E1B" to false
            "sepia" -> "#E9DEC2" to true
            else -> "#F2EDE3" to true
        }
        val color = Color.parseColor(bgHex)
        rootContainer.setBackgroundColor(color)
        webView.setBackgroundColor(color)
        @Suppress("DEPRECATION")
        window.statusBarColor = Color.TRANSPARENT
        @Suppress("DEPRECATION")
        window.navigationBarColor = Color.TRANSPARENT
        WindowCompat.getInsetsController(window, window.decorView).apply {
            isAppearanceLightStatusBars = isLight
            isAppearanceLightNavigationBars = isLight
        }
    }

    private suspend fun downloadAndShare(urlStr: String, contentDisposition: String?, mimeType: String?) {
        val file = withContext(Dispatchers.IO) {
            val exportDir = File(cacheDir, "exports").apply { mkdirs() }
            val filename = when {
                urlStr.contains("/export/epub") -> "inkweave-manuscript.epub"
                urlStr.contains("/export/markdown") -> "inkweave-manuscript.md"
                else -> "inkweave-export.dat"
            }
            val outFile = File(exportDir, filename)
            val conn = URL(urlStr).openConnection() as HttpURLConnection
            engineInfo?.token?.let { tok ->
                conn.setRequestProperty("Authorization", "Bearer $tok")
            }
            conn.inputStream.use { input ->
                outFile.outputStream().use { output ->
                    input.copyTo(output)
                }
            }
            outFile
        }
        val uri: Uri = FileProvider.getUriForFile(this, "${packageName}.fileprovider", file)
        val share = Intent(Intent.ACTION_SEND).apply {
            type = mimeType ?: "*/*"
            putExtra(Intent.EXTRA_STREAM, uri)
            addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
        }
        startActivity(Intent.createChooser(share, "Export Manuscript"))
    }

    inner class NativeBridge {
        @JavascriptInterface
        fun isAndroid(): Boolean = true

        @JavascriptInterface
        fun setTheme(theme: String) {
            runOnUiThread { applyNativeTheme(theme) }
        }

        @JavascriptInterface
        fun getNovelFolder(): String {
            return safSync.getStatus().toJson()
        }

        @JavascriptInterface
        fun pickNovelFolder() {
            runOnUiThread { launchFolderPicker() }
        }

        @JavascriptInterface
        fun syncNovelFolderNow() {
            scope.launch {
                val status = withContext(Dispatchers.IO) {
                    safSync.pullFromSaf()
                    safSync.syncLocalToSaf()
                    safSync.getStatus()
                }
                notifyWebFolderChanged(status, "ซิงก์โฟลเดอร์ Android เรียบร้อยแล้ว")
            }
        }

        @JavascriptInterface
        fun clearNovelFolder() {
            safSync.clearFolder()
            syncLoopJob?.cancel()
            notifyWebFolderChanged(safSync.getStatus(), "ยกเลิกการเชื่อมโฟลเดอร์ภายนอกแล้ว")
        }
    }

    override fun onDestroy() {
        syncLoopJob?.cancel()
        scope.cancel()
        if (isFinishing) {
            Thread {
                runCatching { safSync.syncLocalToSaf() }
                runCatching { engine.stop() }
            }.start()
        }
        super.onDestroy()
    }
}
