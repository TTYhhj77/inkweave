package app.inkweave.studio

import android.content.Context
import android.content.Intent
import android.net.Uri
import android.provider.DocumentsContract
import androidx.documentfile.provider.DocumentFile
import org.json.JSONObject
import java.io.File
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

data class FolderStatus(
    val granted: Boolean,
    val label: String,
    val uri: String,
    val lastSync: String,
) {
    fun toJson(message: String? = null): String {
        val obj = JSONObject()
        obj.put("granted", granted)
        obj.put("label", label)
        obj.put("uri", uri)
        obj.put("lastSync", lastSync)
        if (message != null) {
            obj.put("message", message)
        }
        return obj.toString()
    }
}

/**
 * Two-way Scoped Storage (SAF) mirror between Inkweave's fast local workspace (filesDir/projects)
 * and the user's dedicated Android folder chosen via ACTION_OPEN_DOCUMENT_TREE.
 *
 * Grants read, write, edit, and delete access strictly inside the chosen Android folder —
 * zero broad storage permissions needed.
 */
class SafNovelSync(private val context: Context) {

    private val prefs = context.getSharedPreferences("inkweave_saf_prefs", Context.MODE_PRIVATE)
    private val projectsDir = File(context.filesDir, "projects").apply { mkdirs() }
    private val manifestFile = File(context.filesDir, "saf-sync-manifest.json")
    private val lock = Any()

    companion object {
        private const val KEY_TREE_URI = "novel_tree_uri"
        private const val KEY_LAST_SYNC = "novel_last_sync"
        private const val KEY_PROMPTED = "novel_folder_prompted"
    }

    fun wasPrompted(): Boolean = prefs.getBoolean(KEY_PROMPTED, false)
    fun markPrompted() = prefs.edit().putBoolean(KEY_PROMPTED, true).apply()

    fun getSavedTreeUri(): Uri? {
        val raw = prefs.getString(KEY_TREE_URI, null) ?: return null
        val uri = runCatching { Uri.parse(raw) }.getOrNull() ?: return null
        val hasPerm = context.contentResolver.persistedUriPermissions.any {
            it.uri == uri && it.isReadPermission && it.isWritePermission
        }
        return if (hasPerm) uri else null
    }

    fun getStatus(): FolderStatus {
        val uri = getSavedTreeUri()
        if (uri == null) {
            return FolderStatus(granted = false, label = "", uri = "", lastSync = "")
        }
        val label = friendlyFolderName(uri)
        val last = prefs.getString(KEY_LAST_SYNC, "") ?: ""
        return FolderStatus(granted = true, label = label, uri = uri.toString(), lastSync = last)
    }

    fun clearFolder() {
        prefs.edit().remove(KEY_TREE_URI).remove(KEY_LAST_SYNC).apply()
        if (manifestFile.exists()) manifestFile.delete()
    }

    /**
     * Persists read+write permission on the user-chosen Android folder and performs a safe
     * initial two-way merge (pulling any existing novels in the folder and pushing local ones).
     */
    fun attachFolder(treeUri: Uri): FolderStatus {
        synchronized(lock) {
            val flags = Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_GRANT_WRITE_URI_PERMISSION
            runCatching {
                context.contentResolver.takePersistableUriPermission(treeUri, flags)
            }
            prefs.edit()
                .putString(KEY_TREE_URI, treeUri.toString())
                .putBoolean(KEY_PROMPTED, true)
                .apply()

            val root = DocumentFile.fromTreeUri(context, treeUri)
                ?: return getStatus()

            // 1. Pull any existing novels/files from the chosen Android folder into local projectsDir
            pullTreeRecursive(root, "")

            // 2. Push any local novels/files missing in SAF folder into the chosen Android folder
            val localFiles = scanLocalFiles()
            val manifest = JSONObject()
            for ((rel, file) in localFiles) {
                val existingDoc = findDocument(root, rel)
                if (existingDoc == null) {
                    writeLocalFileToSaf(root, rel, file)
                } else if (file.lastModified() > existingDoc.lastModified() + 2000L) {
                    writeLocalFileToSaf(root, rel, file)
                }
                val entry = JSONObject()
                entry.put("mtime", file.lastModified())
                entry.put("size", file.length())
                manifest.put(rel, entry)
            }
            saveManifest(manifest)
            recordSyncTime()
            return getStatus()
        }
    }

    /**
     * Pulls files that were added or updated externally inside the chosen Android folder.
     * Returns true if at least one file was pulled into local workspace.
     */
    fun pullFromSaf(): Boolean {
        synchronized(lock) {
            val treeUri = getSavedTreeUri() ?: return false
            val root = DocumentFile.fromTreeUri(context, treeUri) ?: return false
            val changed = pullTreeRecursive(root, "")
            if (changed) {
                // Refresh manifest for pulled files
                val manifest = loadManifest()
                for ((rel, file) in scanLocalFiles()) {
                    val entry = JSONObject()
                    entry.put("mtime", file.lastModified())
                    entry.put("size", file.length())
                    manifest.put(rel, entry)
                }
                saveManifest(manifest)
                recordSyncTime()
            }
            return changed
        }
    }

    /**
     * Mirrors local creates, edits, renames, and deletes from filesDir/projects to the
     * user's dedicated Android folder. Ultra-fast no-op when nothing changed.
     */
    fun syncLocalToSaf(): Boolean {
        synchronized(lock) {
            val treeUri = getSavedTreeUri() ?: return false
            val root = DocumentFile.fromTreeUri(context, treeUri) ?: return false

            val prevManifest = loadManifest()
            val localFiles = scanLocalFiles()
            var anyChanges = false

            // 1. Create or update modified local files in SAF folder
            for ((rel, file) in localFiles) {
                val prev = prevManifest.optJSONObject(rel)
                val same = prev != null &&
                    prev.optLong("mtime", -1L) == file.lastModified() &&
                    prev.optLong("size", -1L) == file.length()
                if (!same) {
                    writeLocalFileToSaf(root, rel, file)
                    anyChanges = true
                }
            }

            // 2. Delete files in SAF folder that were removed locally (e.g., deleted chapter/novel)
            val prevKeys = mutableListOf<String>()
            val iter = prevManifest.keys()
            while (iter.hasNext()) {
                prevKeys.add(iter.next())
            }
            val deletedDirs = mutableSetOf<String>()
            for (rel in prevKeys) {
                if (!localFiles.containsKey(rel)) {
                    findDocument(root, rel)?.delete()
                    val slash = rel.lastIndexOf('/')
                    if (slash > 0) {
                        deletedDirs.add(rel.substring(0, slash))
                    }
                    anyChanges = true
                }
            }
            // Prune empty parent directories left behind after deleting a chapter/novel
            for (dirRel in deletedDirs.sortedByDescending { it.length }) {
                pruneEmptySafDir(root, dirRel)
            }

            if (anyChanges) {
                val nextManifest = JSONObject()
                for ((rel, file) in localFiles) {
                    val entry = JSONObject()
                    entry.put("mtime", file.lastModified())
                    entry.put("size", file.length())
                    nextManifest.put(rel, entry)
                }
                saveManifest(nextManifest)
                recordSyncTime()
            }
            return anyChanges
        }
    }

    private fun shouldIgnore(relPath: String): Boolean {
        val norm = relPath.replace('\\', '/').trimStart('/')
        if (norm.isEmpty()) return true
        val segments = norm.split('/')
        for (seg in segments) {
            if (seg == ".inkweave" || seg == ".git" || seg == ".DS_Store") return true
            if (seg.endsWith(".tmp") || seg.endsWith(".sqlite") || seg.endsWith("-wal") || seg.endsWith("-shm")) {
                return true
            }
        }
        return false
    }

    private fun scanLocalFiles(): Map<String, File> {
        val out = LinkedHashMap<String, File>()
        if (!projectsDir.exists()) return out
        fun walk(dir: File, prefix: String) {
            val list = dir.listFiles() ?: return
            for (f in list) {
                val rel = if (prefix.isEmpty()) f.name else "$prefix/${f.name}"
                if (shouldIgnore(rel)) continue
                if (f.isDirectory) {
                    walk(f, rel)
                } else if (f.isFile) {
                    out[rel] = f
                }
            }
        }
        walk(projectsDir, "")
        return out
    }

    private fun pullTreeRecursive(dir: DocumentFile, prefix: String): Boolean {
        var changed = false
        for (child in dir.listFiles()) {
            val name = child.name ?: continue
            val rel = if (prefix.isEmpty()) name else "$prefix/$name"
            if (shouldIgnore(rel)) continue
            if (child.isDirectory) {
                if (pullTreeRecursive(child, rel)) changed = true
            } else if (child.isFile) {
                val localTarget = File(projectsDir, rel)
                val shouldCopy = !localTarget.exists() ||
                    child.lastModified() > localTarget.lastModified() + 2000L
                if (shouldCopy) {
                    localTarget.parentFile?.mkdirs()
                    context.contentResolver.openInputStream(child.uri)?.use { input ->
                        localTarget.outputStream().use { output ->
                            input.copyTo(output)
                        }
                    }
                    if (child.lastModified() > 0L) {
                        localTarget.setLastModified(child.lastModified())
                    }
                    changed = true
                }
            }
        }
        return changed
    }

    private fun writeLocalFileToSaf(root: DocumentFile, relPath: String, localFile: File) {
        val parts = relPath.split('/').filter { it.isNotEmpty() }
        if (parts.isEmpty()) return
        var cur = root
        for (i in 0 until parts.size - 1) {
            val seg = parts[i]
            val existing = cur.findFile(seg)
            cur = if (existing != null && existing.isDirectory) {
                existing
            } else {
                cur.createDirectory(seg) ?: return
            }
        }
        val fileName = parts.last()
        var target = cur.findFile(fileName)
        if (target == null) {
            val mime = guessMime(fileName)
            target = cur.createFile(mime, fileName) ?: return
        }
        context.contentResolver.openOutputStream(target.uri, "wt")?.use { out ->
            localFile.inputStream().use { inp ->
                inp.copyTo(out)
            }
        }
    }

    private fun findDocument(root: DocumentFile, relPath: String): DocumentFile? {
        val parts = relPath.split('/').filter { it.isNotEmpty() }
        var cur: DocumentFile? = root
        for (seg in parts) {
            cur = cur?.findFile(seg) ?: return null
        }
        return cur
    }

    private fun pruneEmptySafDir(root: DocumentFile, dirRel: String) {
        val parts = dirRel.split('/').filter { it.isNotEmpty() }.toMutableList()
        while (parts.isNotEmpty()) {
            val path = parts.joinToString("/")
            val doc = findDocument(root, path) ?: break
            if (doc.isDirectory && doc.listFiles().isEmpty()) {
                doc.delete()
                parts.removeAt(parts.lastIndex)
            } else {
                break
            }
        }
    }

    private fun guessMime(fileName: String): String {
        val lower = fileName.lowercase(Locale.ROOT)
        return when {
            lower.endsWith(".md") -> "text/markdown"
            lower.endsWith(".yaml") || lower.endsWith(".yml") -> "text/plain"
            lower.endsWith(".json") -> "application/json"
            else -> "application/octet-stream"
        }
    }

    private fun friendlyFolderName(treeUri: Uri): String {
        val docId = runCatching { DocumentsContract.getTreeDocumentId(treeUri) }.getOrNull()
        if (!docId.isNullOrBlank()) {
            val cleaned = docId.substringAfter(':', docId)
            if (cleaned.isNotBlank()) return cleaned
        }
        return DocumentFile.fromTreeUri(context, treeUri)?.name ?: "Inkweave Folder"
    }

    private fun loadManifest(): JSONObject {
        if (!manifestFile.exists()) return JSONObject()
        return runCatching { JSONObject(manifestFile.readText()) }.getOrDefault(JSONObject())
    }

    private fun saveManifest(obj: JSONObject) {
        val tmp = File(manifestFile.parentFile, manifestFile.name + ".tmp")
        tmp.writeText(obj.toString())
        tmp.renameTo(manifestFile)
    }

    private fun recordSyncTime() {
        val fmt = SimpleDateFormat("HH:mm:ss", Locale.getDefault())
        prefs.edit().putString(KEY_LAST_SYNC, fmt.format(Date())).apply()
    }
}
