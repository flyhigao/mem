package com.flyhigao.mem.ui

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.provider.Settings
import android.widget.Toast
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ContentPaste
import androidx.compose.material.icons.filled.DeleteOutline
import androidx.compose.material.icons.filled.Layers
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Send
import androidx.compose.material.icons.filled.Settings
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
    var isLoading by remember { mutableStateOf(false) }
    var isSending by remember { mutableStateOf(false) }
    var showSettingsDialog by remember { mutableStateOf(false) }
    var isFloatingRunning by remember { mutableStateOf(FloatingInputService.isRunning) }

    // Load initial settings
    LaunchedEffect(Unit) {
        serverUrl = prefs.getServerUrl()
        apiToken = prefs.getApiToken()
        if (apiToken.isNotEmpty()) {
            loadMessages(serverUrl, apiToken) { list -> messages = list }
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
                            "中转站",
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
                .padding(horizontal = 16.dp, vertical = 12.dp)
        ) {
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
                            .heightIn(min = 100.dp, max = 180.dp),
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
                            // Paste clipboard button
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

                            // Clear button
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

                        // Send button
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
                                Icon(Icons.Default.Send, contentDescription = null, modifier = Modifier.size(16.dp))
                                Spacer(modifier = Modifier.width(6.dp))
                                Text("发送", fontSize = 13.sp, fontWeight = FontWeight.Bold)
                            }
                        }
                    }
                }
            }

            Spacer(modifier = Modifier.height(16.dp))

            // History header
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically
            ) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text("📋 历史记录", fontWeight = FontWeight.Bold, fontSize = 15.sp, color = Color(0xFFF8FAFC))
                    Spacer(modifier = Modifier.width(8.dp))
                    Text(
                        "${messages.size} 条",
                        fontSize = 11.sp,
                        color = MaterialTheme.colorScheme.primary,
                        modifier = Modifier
                            .background(Color(0xFF38BDF8).copy(alpha = 0.15f), RoundedCornerShape(99.dp))
                            .padding(horizontal = 8.dp, vertical = 2.dp)
                    )
                }

                IconButton(
                    onClick = {
                        isLoading = true
                        scope.launch {
                            loadMessages(serverUrl, apiToken) { list ->
                                messages = list
                                isLoading = false
                            }
                        }
                    }
                ) {
                    Icon(Icons.Default.Refresh, contentDescription = "刷新", tint = Color(0xFF94A3B8))
                }
            }

            Spacer(modifier = Modifier.height(8.dp))

            // Message list
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
                            loadMessages(serverUrl, apiToken) { list -> messages = list }
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

private fun loadMessages(serverUrl: String, token: String, onLoaded: (List<Message>) -> Unit) {
    kotlinx.coroutines.CoroutineScope(kotlinx.coroutines.Dispatchers.Main).launch {
        val result = MemApiClient.getMessages(serverUrl, token, 100)
        if (result.isSuccess) {
            onLoaded(result.getOrDefault(emptyList()))
        }
    }
}
