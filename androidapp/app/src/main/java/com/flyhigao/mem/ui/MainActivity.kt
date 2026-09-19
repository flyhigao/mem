package com.flyhigao.mem.ui

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.provider.OpenableColumns
import android.provider.Settings
import android.widget.Toast
import androidx.activity.ComponentActivity
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.Send
import androidx.compose.material.icons.filled.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.flyhigao.mem.data.FileRecord
import com.flyhigao.mem.data.MemApiClient
import com.flyhigao.mem.data.Message
import com.flyhigao.mem.data.PreferencesManager
import com.flyhigao.mem.service.FloatingInputService
import kotlinx.coroutines.launch

class MainActivity : ComponentActivity() {

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        setContent {
            MaterialTheme(
                colorScheme = darkColorScheme(
                    primary = Color(0xFF38BDF8),
                    background = Color(0xFF0F172A),
                    surface = Color(0xFF1E293B),
                    onPrimary = Color.White,
                    onBackground = Color(0xFFF8FAFC),
                    onSurface = Color(0xFFF8FAFC)
                )
            ) {
                Surface(
                    modifier = Modifier.fillMaxSize(),
                    color = MaterialTheme.colorScheme.background
                ) {
                    MemAppScreen()
                }
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun MemAppScreen() {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val prefs = remember { PreferencesManager(context) }

    var serverUrl by remember { mutableStateOf("") }
    var apiToken by remember { mutableStateOf("") }
    var inputText by remember { mutableStateOf("") }
    var messages by remember { mutableStateOf<List<Message>>(emptyList()) }
    var files by remember { mutableStateOf<List<FileRecord>>(emptyList()) }
    var totalFileSize by remember { mutableStateOf(0L) }
    var maxFileSize by remember { mutableStateOf(1024L * 1024L * 1024L) }

    var currentTab by remember { mutableStateOf(0) } // 0: 文本, 1: 文件
    var isLoading by remember { mutableStateOf(false) }
    var isSending by remember { mutableStateOf(false) }
    var isUploading by remember { mutableStateOf(false) }
    var uploadStatusText by remember { mutableStateOf("") }
    var showSettingsDialog by remember { mutableStateOf(false) }
    var isFloatingRunning by remember { mutableStateOf(FloatingInputService.isRunning) }

    fun refreshAll() {
        if (apiToken.isNotEmpty()) {
            isLoading = true
            scope.launch {
                loadMessages(serverUrl, apiToken) { messages = it }
                loadFiles(serverUrl, apiToken) { resp ->
                    files = resp.files
                    totalFileSize = resp.totalSize
                    maxFileSize = resp.maxSize
                }
                isLoading = false
            }
        }
    }

    // File picker launcher (supports multiple selection)
    val filePickerLauncher = rememberLauncherForActivityResult(
        contract = ActivityResultContracts.GetMultipleContents()
    ) { uris ->
        if (uris.isNotEmpty()) {
            if (apiToken.isEmpty()) {
                Toast.makeText(context, "请先在右上角设置中填写 Token", Toast.LENGTH_LONG).show()
                showSettingsDialog = true
                return@rememberLauncherForActivityResult
            }

            scope.launch {
                isUploading = true
                val maxSingle = 10 * 1024 * 1024L // 10MB
                val filesToUpload = mutableListOf<Pair<String, ByteArray>>()

                for (uri in uris) {
                    val (name, size) = getFileInfo(context, uri)
                    if (size > maxSingle) {
                        Toast.makeText(context, "文件 [$name] 超过 10MB 限制，跳过", Toast.LENGTH_LONG).show()
                        continue
                    }
                    try {
                        val bytes = context.contentResolver.openInputStream(uri)?.use { it.readBytes() }
                        if (bytes != null) {
                            if (bytes.size > maxSingle) {
                                Toast.makeText(context, "文件 [$name] 超过 10MB 限制，跳过", Toast.LENGTH_LONG).show()
                            } else {
                                filesToUpload.add(Pair(name, bytes))
                            }
                        }
                    } catch (e: Exception) {
                        Toast.makeText(context, "无法读取文件 [$name]: ${e.message}", Toast.LENGTH_SHORT).show()
                    }
                }

                if (filesToUpload.isEmpty()) {
                    isUploading = false
                    return@launch
                }

                uploadStatusText = "正在上传 ${filesToUpload.size} 个文件..."
                val result = MemApiClient.uploadFiles(serverUrl, apiToken, filesToUpload)
                isUploading = false
                uploadStatusText = ""

                if (result.isSuccess) {
                    Toast.makeText(context, "✅ 成功上传 ${filesToUpload.size} 个文件", Toast.LENGTH_SHORT).show()
                    loadFiles(serverUrl, apiToken) { resp ->
                        files = resp.files
                        totalFileSize = resp.totalSize
                        maxFileSize = resp.maxSize
                    }
                } else {
                    val err = result.exceptionOrNull()?.message ?: "上传失败"
                    Toast.makeText(context, "❌ 上传失败: $err", Toast.LENGTH_LONG).show()
                }
            }
        }
    }

    // Load initial settings
    LaunchedEffect(Unit) {
        serverUrl = prefs.getServerUrl()
        apiToken = prefs.getApiToken()
        if (apiToken.isNotEmpty()) {
            refreshAll()
        } else {
            showSettingsDialog = true
        }
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Text(
                            "⚡ Mem",
                            fontWeight = FontWeight.Bold,
                            color = MaterialTheme.colorScheme.primary,
                            fontSize = 20.sp
                        )
                        Spacer(modifier = Modifier.width(8.dp))
                        Text(
                            "跨端中转站",
                            fontSize = 14.sp,
                            color = Color(0xFF94A3B8)
                        )
                    }
                },
                actions = {
                    // Floating window button
                    IconButton(onClick = {
                        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.M && !Settings.canDrawOverlays(context)) {
                            val intent = Intent(
                                Settings.ACTION_MANAGE_OVERLAY_PERMISSION,
                                Uri.parse("package:${context.packageName}")
                            )
                            context.startActivity(intent)
                            Toast.makeText(context, "请先授予 Mem 悬浮窗权限", Toast.LENGTH_LONG).show()
                            return@IconButton
                        }

                        val intent = Intent(context, FloatingInputService::class.java)
                        if (isFloatingRunning) {
                            context.stopService(intent)
                            isFloatingRunning = false
                            Toast.makeText(context, "悬浮窗已关闭", Toast.LENGTH_SHORT).show()
                        } else {
                            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                                context.startForegroundService(intent)
                            } else {
                                context.startService(intent)
                            }
                            isFloatingRunning = true
                            Toast.makeText(context, "悬浮窗已开启", Toast.LENGTH_SHORT).show()
                        }
                    }) {
                        Icon(
                            Icons.Default.Layers,
                            contentDescription = "悬浮窗",
                            tint = if (isFloatingRunning) Color(0xFF10B981) else Color(0xFF94A3B8)
                        )
                    }

                    // Settings button
                    IconButton(onClick = { showSettingsDialog = true }) {
                        Icon(Icons.Default.Settings, contentDescription = "设置", tint = Color(0xFF94A3B8))
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = Color(0xFF1E293B)
                )
            )
        }
    ) { innerPadding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(innerPadding)
        ) {
            // Tab Row
            PrimaryTabRow(
                selectedTabIndex = currentTab,
                containerColor = Color(0xFF1E293B),
                contentColor = MaterialTheme.colorScheme.primary
            ) {
                Tab(
                    selected = currentTab == 0,
                    onClick = { currentTab = 0 },
                    text = {
                        Text(
                            "📝 文本 (${messages.size})",
                            fontSize = 14.sp,
                            fontWeight = if (currentTab == 0) FontWeight.Bold else FontWeight.Normal
                        )
                    }
                )
                Tab(
                    selected = currentTab == 1,
                    onClick = {
                        currentTab = 1
                        scope.launch {
                            loadFiles(serverUrl, apiToken) { resp ->
                                files = resp.files
                                totalFileSize = resp.totalSize
                                maxFileSize = resp.maxSize
                            }
                        }
                    },
                    text = {
                        Text(
                            "📁 文件 (${files.size})",
                            fontSize = 14.sp,
                            fontWeight = if (currentTab == 1) FontWeight.Bold else FontWeight.Normal
                        )
                    }
                )
            }

            Column(
                modifier = Modifier
                    .fillMaxSize()
                    .padding(horizontal = 16.dp, vertical = 12.dp)
            ) {
                if (currentTab == 0) {
                    // === TAB 0: TEXT MESSAGES ===

                    // Input Card
                    Card(
                        colors = CardDefaults.cardColors(containerColor = Color(0xFF1E293B)),
                        shape = RoundedCornerShape(12.dp),
                        modifier = Modifier.fillMaxWidth()
                    ) {
                        Column(modifier = Modifier.padding(14.dp)) {
                            OutlinedTextField(
                                value = inputText,
                                onValueChange = { inputText = it },
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .heightIn(min = 90.dp, max = 160.dp),
                                placeholder = { Text("在此输入或粘贴文字...", color = Color(0xFF64748B), fontSize = 14.sp) },
                                colors = OutlinedTextFieldDefaults.colors(
                                    focusedBorderColor = MaterialTheme.colorScheme.primary,
                                    unfocusedBorderColor = Color(0xFF334155),
                                    focusedTextColor = Color.White,
                                    unfocusedTextColor = Color.White
                                )
                            )

                            Spacer(modifier = Modifier.height(10.dp))

                            Row(
                                modifier = Modifier.fillMaxWidth(),
                                horizontalArrangement = Arrangement.SpaceBetween,
                                verticalAlignment = Alignment.CenterVertically
                            ) {
                                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                                    OutlinedButton(
                                        onClick = {
                                            val clipManager = context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
                                            val clip = clipManager.primaryClip
                                            if (clip != null && clip.itemCount > 0) {
                                                inputText += clip.getItemAt(0).text?.toString() ?: ""
                                            } else {
                                                Toast.makeText(context, "剪贴板为空", Toast.LENGTH_SHORT).show()
                                            }
                                        },
                                        contentPadding = PaddingValues(horizontal = 10.dp, vertical = 6.dp),
                                        shape = RoundedCornerShape(8.dp)
                                    ) {
                                        Icon(Icons.Default.ContentPaste, contentDescription = null, modifier = Modifier.size(16.dp))
                                        Spacer(modifier = Modifier.width(4.dp))
                                        Text("粘贴", fontSize = 12.sp)
                                    }

                                    if (inputText.isNotEmpty()) {
                                        OutlinedButton(
                                            onClick = { inputText = "" },
                                            contentPadding = PaddingValues(horizontal = 10.dp, vertical = 6.dp),
                                            shape = RoundedCornerShape(8.dp)
                                        ) {
                                            Icon(Icons.Default.DeleteOutline, contentDescription = null, modifier = Modifier.size(16.dp))
                                            Spacer(modifier = Modifier.width(4.dp))
                                            Text("清空", fontSize = 12.sp)
                                        }
                                    }
                                }

                                Button(
                                    onClick = {
                                        if (inputText.trim().isEmpty()) {
                                            Toast.makeText(context, "内容不能为空", Toast.LENGTH_SHORT).show()
                                            return@Button
                                        }
                                        if (apiToken.isEmpty()) {
                                            Toast.makeText(context, "请先在右上角设置中填写 Token", Toast.LENGTH_LONG).show()
                                            showSettingsDialog = true
                                            return@Button
                                        }

                                        isSending = true
                                        scope.launch {
                                            val result = MemApiClient.sendMessage(serverUrl, apiToken, inputText.trim(), "Android")
                                            isSending = false
                                            if (result.isSuccess) {
                                                Toast.makeText(context, "🚀 发送成功！", Toast.LENGTH_SHORT).show()
                                                inputText = ""
                                                loadMessages(serverUrl, apiToken) { list -> messages = list }
                                            } else {
                                                val err = result.exceptionOrNull()?.message ?: "发送失败"
                                                Toast.makeText(context, "❌ $err", Toast.LENGTH_LONG).show()
                                            }
                                        }
                                    },
                                    enabled = !isSending,
                                    colors = ButtonDefaults.buttonColors(containerColor = MaterialTheme.colorScheme.primary),
                                    shape = RoundedCornerShape(8.dp)
                                ) {
                                    if (isSending) {
                                        CircularProgressIndicator(modifier = Modifier.size(16.dp), color = Color.White, strokeWidth = 2.dp)
                                        Spacer(modifier = Modifier.width(6.dp))
                                        Text("发送中", fontSize = 13.sp)
                                    } else {
                                        Icon(Icons.AutoMirrored.Filled.Send, contentDescription = null, modifier = Modifier.size(16.dp))
                                        Spacer(modifier = Modifier.width(6.dp))
                                        Text("发送", fontSize = 13.sp, fontWeight = FontWeight.Bold)
                                    }
                                }
                            }
                        }
                    }

                    Spacer(modifier = Modifier.height(14.dp))

                    // Message list header
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.SpaceBetween,
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Text("📋 文本记录 (最新 500 条)", fontWeight = FontWeight.Bold, fontSize = 15.sp, color = Color(0xFFF8FAFC))
                        IconButton(
                            onClick = {
                                isLoading = true
                                scope.launch {
                                    loadMessages(serverUrl, apiToken) {
                                        messages = it
                                        isLoading = false
                                    }
                                }
                            }
                        ) {
                            Icon(Icons.Default.Refresh, contentDescription = "刷新", tint = Color(0xFF94A3B8))
                        }
                    }

                    Spacer(modifier = Modifier.height(6.dp))

                    if (messages.isEmpty()) {
                        Box(
                            modifier = Modifier
                                .fillMaxWidth()
                                .weight(1f),
                            contentAlignment = Alignment.Center
                        ) {
                            Text("暂无消息记录", color = Color(0xFF64748B), fontSize = 14.sp)
                        }
                    } else {
                        LazyColumn(
                            modifier = Modifier.weight(1f),
                            verticalArrangement = Arrangement.spacedBy(10.dp)
                        ) {
                            items(messages, key = { it.id }) { msg ->
                                MessageCard(msg = msg, onCopy = {
                                    val clipManager = context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
                                    val clip = ClipData.newPlainText("mem_text", msg.content)
                                    clipManager.setPrimaryClip(clip)
                                    Toast.makeText(context, "✅ 已复制到剪贴板", Toast.LENGTH_SHORT).show()
                                })
                            }
                        }
                    }
                } else {
                    // === TAB 1: FILES ===

                    // Storage & Upload Card
                    Card(
                        colors = CardDefaults.cardColors(containerColor = Color(0xFF1E293B)),
                        shape = RoundedCornerShape(12.dp),
                        modifier = Modifier.fillMaxWidth()
                    ) {
                        Column(modifier = Modifier.padding(14.dp)) {
                            Row(
                                modifier = Modifier.fillMaxWidth(),
                                horizontalArrangement = Arrangement.SpaceBetween,
                                verticalAlignment = Alignment.CenterVertically
                            ) {
                                Text("💾 存储配额 (上限 1 GB)", fontWeight = FontWeight.Bold, fontSize = 14.sp, color = Color(0xFFF8FAFC))
                                val usedMB = totalFileSize / (1024.0 * 1024.0)
                                val percent = if (maxFileSize > 0) (totalFileSize * 100.0 / maxFileSize).coerceIn(0.0, 100.0) else 0.0
                                Text(
                                    String.format("%.1f MB / 1024 MB (%.1f%%)", usedMB, percent),
                                    fontSize = 12.sp,
                                    color = Color(0xFF94A3B8)
                                )
                            }

                            Spacer(modifier = Modifier.height(6.dp))

                            val progress = if (maxFileSize > 0) (totalFileSize.toFloat() / maxFileSize.toFloat()).coerceIn(0f, 1f) else 0f
                            LinearProgressIndicator(
                                progress = { progress },
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .height(6.dp),
                                color = MaterialTheme.colorScheme.primary,
                                trackColor = Color(0xFF0F172A),
                            )

                            Spacer(modifier = Modifier.height(8.dp))

                            Text(
                                "💡 单文件上限 10MB，超额自动淘汰最旧文件",
                                fontSize = 11.sp,
                                color = Color(0xFF64748B)
                            )

                            Spacer(modifier = Modifier.height(12.dp))

                            // Upload Button
                            Button(
                                onClick = {
                                    filePickerLauncher.launch("*/*")
                                },
                                enabled = !isUploading,
                                modifier = Modifier.fillMaxWidth(),
                                shape = RoundedCornerShape(8.dp),
                                colors = ButtonDefaults.buttonColors(containerColor = MaterialTheme.colorScheme.primary)
                            ) {
                                if (isUploading) {
                                    CircularProgressIndicator(modifier = Modifier.size(16.dp), color = Color.White, strokeWidth = 2.dp)
                                    Spacer(modifier = Modifier.width(8.dp))
                                    Text(if (uploadStatusText.isNotEmpty()) uploadStatusText else "正在上传...", fontSize = 13.sp)
                                } else {
                                    Icon(Icons.Default.CloudUpload, contentDescription = null, modifier = Modifier.size(18.dp))
                                    Spacer(modifier = Modifier.width(8.dp))
                                    Text("选择文件上传 (支持多选)", fontSize = 14.sp, fontWeight = FontWeight.Bold)
                                }
                            }
                        }
                    }

                    Spacer(modifier = Modifier.height(14.dp))

                    // Files list header
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.SpaceBetween,
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Text("📁 已上传文件列表", fontWeight = FontWeight.Bold, fontSize = 15.sp, color = Color(0xFFF8FAFC))
                        IconButton(
                            onClick = {
                                isLoading = true
                                scope.launch {
                                    loadFiles(serverUrl, apiToken) { resp ->
                                        files = resp.files
                                        totalFileSize = resp.totalSize
                                        maxFileSize = resp.maxSize
                                        isLoading = false
                                    }
                                }
                            }
                        ) {
                            Icon(Icons.Default.Refresh, contentDescription = "刷新", tint = Color(0xFF94A3B8))
                        }
                    }

                    Spacer(modifier = Modifier.height(6.dp))

                    if (files.isEmpty()) {
                        Box(
                            modifier = Modifier
                                .fillMaxWidth()
                                .weight(1f),
                            contentAlignment = Alignment.Center
                        ) {
                            Text("暂无上传的文件", color = Color(0xFF64748B), fontSize = 14.sp)
                        }
                    } else {
                        LazyColumn(
                            modifier = Modifier.weight(1f),
                            verticalArrangement = Arrangement.spacedBy(10.dp)
                        ) {
                            items(files, key = { it.id }) { file ->
                                val downloadUrl = "${serverUrl.trimEnd('/')}/api/v1/files/${file.id}/download?token=${apiToken}"

                                FileCard(
                                    file = file,
                                    onCopyLink = {
                                        val clipManager = context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
                                        val clip = ClipData.newPlainText("mem_file_url", downloadUrl)
                                        clipManager.setPrimaryClip(clip)
                                        Toast.makeText(context, "✅ 已复制下载直链", Toast.LENGTH_SHORT).show()
                                    },
                                    onDownload = {
                                        try {
                                            val intent = Intent(Intent.ACTION_VIEW, Uri.parse(downloadUrl))
                                            context.startActivity(intent)
                                        } catch (e: Exception) {
                                            Toast.makeText(context, "打开下载失败: ${e.message}", Toast.LENGTH_SHORT).show()
                                        }
                                    },
                                    onDelete = {
                                        scope.launch {
                                            val res = MemApiClient.deleteFile(serverUrl, apiToken, file.id)
                                            if (res.isSuccess) {
                                                Toast.makeText(context, "🗑️ 文件已删除", Toast.LENGTH_SHORT).show()
                                                loadFiles(serverUrl, apiToken) { resp ->
                                                    files = resp.files
                                                    totalFileSize = resp.totalSize
                                                    maxFileSize = resp.maxSize
                                                }
                                            } else {
                                                Toast.makeText(context, "删除失败: ${res.exceptionOrNull()?.message}", Toast.LENGTH_SHORT).show()
                                            }
                                        }
                                    }
                                )
                            }
                        }
                    }
                }
            }
        }
    }

    // Settings Dialog
    if (showSettingsDialog) {
        var tempUrl by remember { mutableStateOf(serverUrl) }
        var tempToken by remember { mutableStateOf(apiToken) }
        var testResult by remember { mutableStateOf<String?>(null) }
        var isTesting by remember { mutableStateOf(false) }

        AlertDialog(
            onDismissRequest = { showSettingsDialog = false },
            title = { Text("⚙️ 服务配置", fontWeight = FontWeight.Bold, fontSize = 18.sp) },
            text = {
                Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                    Text(
                        "请配置 Web 中转服务器地址和登录后生成的 API Token：",
                        fontSize = 12.sp,
                        color = Color(0xFF94A3B8)
                    )

                    OutlinedTextField(
                        value = tempUrl,
                        onValueChange = { tempUrl = it },
                        label = { Text("服务器 URL") },
                        placeholder = { Text("http://192.168.2.97:8080") },
                        singleLine = true,
                        modifier = Modifier.fillMaxWidth()
                    )

                    OutlinedTextField(
                        value = tempToken,
                        onValueChange = { tempToken = it },
                        label = { Text("API Token") },
                        placeholder = { Text("mem_...") },
                        singleLine = true,
                        modifier = Modifier.fillMaxWidth()
                    )

                    if (testResult != null) {
                        Text(
                            text = testResult!!,
                            fontSize = 12.sp,
                            color = if (testResult!!.startsWith("连接成功")) Color(0xFF10B981) else Color(0xFFF43F5E)
                        )
                    }

                    OutlinedButton(
                        onClick = {
                            isTesting = true
                            testResult = null
                            scope.launch {
                                val res = MemApiClient.testConnection(tempUrl, tempToken)
                                isTesting = false
                                testResult = if (res.isSuccess) {
                                    res.getOrNull()
                                } else {
                                    res.exceptionOrNull()?.message ?: "连接失败"
                                }
                            }
                        },
                        enabled = !isTesting,
                        modifier = Modifier.fillMaxWidth()
                    ) {
                        if (isTesting) {
                            CircularProgressIndicator(modifier = Modifier.size(14.dp), strokeWidth = 2.dp)
                            Spacer(modifier = Modifier.width(6.dp))
                        }
                        Text("测试连接", fontSize = 13.sp)
                    }
                }
            },
            confirmButton = {
                Button(
                    onClick = {
                        scope.launch {
                            prefs.saveServerUrl(tempUrl)
                            prefs.saveApiToken(tempToken)
                            serverUrl = tempUrl
                            apiToken = tempToken
                            showSettingsDialog = false
                            refreshAll()
                        }
                    }
                ) {
                    Text("保存")
                }
            },
            dismissButton = {
                TextButton(onClick = { showSettingsDialog = false }) {
                    Text("取消")
                }
            },
            containerColor = Color(0xFF1E293B)
        )
    }
}

@Composable
fun MessageCard(msg: Message, onCopy: () -> Unit) {
    Card(
        colors = CardDefaults.cardColors(containerColor = Color(0xFF1E293B)),
        shape = RoundedCornerShape(10.dp),
        modifier = Modifier.fillMaxWidth()
    ) {
        Column(modifier = Modifier.padding(12.dp)) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically
            ) {
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    val src = msg.source ?: "web"
                    val tagBg = when {
                        src.contains("android", ignoreCase = true) -> Color(0xFF059669)
                        src.contains("linux", ignoreCase = true) -> Color(0xFFD97706)
                        else -> Color(0xFF2563EB)
                    }
                    Text(
                        text = src.uppercase(),
                        fontSize = 10.sp,
                        fontWeight = FontWeight.Bold,
                        color = Color.White,
                        modifier = Modifier
                            .background(tagBg, RoundedCornerShape(4.dp))
                            .padding(horizontal = 6.dp, vertical = 2.dp)
                    )

                    if (!msg.tokenName.isNullOrEmpty()) {
                        Text(
                            text = "🔑 ${msg.tokenName}",
                            fontSize = 10.sp,
                            color = Color(0xFF94A3B8)
                        )
                    }
                }

                Text(
                    text = msg.createdAt.replace("T", " ").take(19),
                    fontSize = 11.sp,
                    color = Color(0xFF64748B)
                )
            }

            Spacer(modifier = Modifier.height(8.dp))

            Text(
                text = msg.content,
                fontSize = 14.sp,
                color = Color(0xFFF8FAFC),
                lineHeight = 20.sp,
                fontFamily = FontFamily.Default
            )

            Spacer(modifier = Modifier.height(8.dp))

            Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
                TextButton(
                    onClick = onCopy,
                    contentPadding = PaddingValues(horizontal = 10.dp, vertical = 4.dp)
                ) {
                    Icon(Icons.Default.ContentPaste, contentDescription = null, modifier = Modifier.size(14.dp))
                    Spacer(modifier = Modifier.width(4.dp))
                    Text("复制", fontSize = 12.sp)
                }
            }
        }
    }
}

@Composable
fun FileCard(
    file: FileRecord,
    onCopyLink: () -> Unit,
    onDownload: () -> Unit,
    onDelete: () -> Unit
) {
    Card(
        colors = CardDefaults.cardColors(containerColor = Color(0xFF1E293B)),
        shape = RoundedCornerShape(10.dp),
        modifier = Modifier.fillMaxWidth()
    ) {
        Column(modifier = Modifier.padding(12.dp)) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically
            ) {
                Icon(
                    Icons.Default.InsertDriveFile,
                    contentDescription = null,
                    tint = MaterialTheme.colorScheme.primary,
                    modifier = Modifier.size(28.dp)
                )
                Spacer(modifier = Modifier.width(10.dp))
                Column(modifier = Modifier.weight(1f)) {
                    Text(
                        text = file.filename,
                        fontSize = 14.sp,
                        fontWeight = FontWeight.SemiBold,
                        color = Color(0xFFF8FAFC),
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis
                    )
                    Spacer(modifier = Modifier.height(2.dp))
                    Row(
                        horizontalArrangement = Arrangement.spacedBy(8.dp),
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Text(
                            text = formatBytes(file.fileSize),
                            fontSize = 11.sp,
                            color = MaterialTheme.colorScheme.primary
                        )
                        Text(
                            text = file.createdAt.replace("T", " ").take(19),
                            fontSize = 11.sp,
                            color = Color(0xFF64748B)
                        )
                        if (!file.tokenName.isNullOrEmpty()) {
                            Text(
                                text = "🔑 ${file.tokenName}",
                                fontSize = 11.sp,
                                color = Color(0xFF94A3B8)
                            )
                        }
                    }
                }
            }

            Spacer(modifier = Modifier.height(8.dp))

            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.End,
                verticalAlignment = Alignment.CenterVertically
            ) {
                TextButton(
                    onClick = onCopyLink,
                    contentPadding = PaddingValues(horizontal = 8.dp, vertical = 2.dp)
                ) {
                    Icon(Icons.Default.Link, contentDescription = null, modifier = Modifier.size(14.dp))
                    Spacer(modifier = Modifier.width(4.dp))
                    Text("直链", fontSize = 12.sp)
                }

                TextButton(
                    onClick = onDownload,
                    contentPadding = PaddingValues(horizontal = 8.dp, vertical = 2.dp)
                ) {
                    Icon(Icons.Default.FileDownload, contentDescription = null, modifier = Modifier.size(14.dp))
                    Spacer(modifier = Modifier.width(4.dp))
                    Text("下载", fontSize = 12.sp)
                }

                TextButton(
                    onClick = onDelete,
                    contentPadding = PaddingValues(horizontal = 8.dp, vertical = 2.dp),
                    colors = ButtonDefaults.textButtonColors(contentColor = Color(0xFFF43F5E))
                ) {
                    Icon(Icons.Default.DeleteOutline, contentDescription = null, modifier = Modifier.size(14.dp))
                    Spacer(modifier = Modifier.width(4.dp))
                    Text("删除", fontSize = 12.sp)
                }
            }
        }
    }
}

private fun formatBytes(bytes: Long): String {
    if (bytes <= 0) return "0 B"
    val k = 1024.0
    val sizes = arrayOf("B", "KB", "MB", "GB")
    val i = (Math.log(bytes.toDouble()) / Math.log(k)).toInt().coerceIn(0, 3)
    return String.format("%.2f %s", bytes / Math.pow(k, i.toDouble()), sizes[i])
}

private fun getFileInfo(context: Context, uri: Uri): Pair<String, Long> {
    var name = "file"
    var size = 0L
    try {
        context.contentResolver.query(uri, null, null, null, null)?.use { cursor ->
            val nameIndex = cursor.getColumnIndex(OpenableColumns.DISPLAY_NAME)
            val sizeIndex = cursor.getColumnIndex(OpenableColumns.SIZE)
            if (cursor.moveToFirst()) {
                if (nameIndex != -1) name = cursor.getString(nameIndex) ?: "file"
                if (sizeIndex != -1) size = cursor.getLong(sizeIndex)
            }
        }
    } catch (e: Exception) {
        // Fallback
    }
    return Pair(name, size)
}

private fun loadMessages(serverUrl: String, token: String, onLoaded: (List<Message>) -> Unit) {
    kotlinx.coroutines.CoroutineScope(kotlinx.coroutines.Dispatchers.Main).launch {
        val result = MemApiClient.getMessages(serverUrl, token, 100)
        if (result.isSuccess) {
            onLoaded(result.getOrDefault(emptyList()))
        }
    }
}

private fun loadFiles(serverUrl: String, token: String, onLoaded: (com.flyhigao.mem.data.FileListResponse) -> Unit) {
    kotlinx.coroutines.CoroutineScope(kotlinx.coroutines.Dispatchers.Main).launch {
        val result = MemApiClient.getFiles(serverUrl, token, 100)
        if (result.isSuccess) {
            result.getOrNull()?.let { onLoaded(it) }
        }
    }
}
