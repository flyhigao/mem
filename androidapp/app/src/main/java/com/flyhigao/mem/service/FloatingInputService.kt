package com.flyhigao.mem.service

import android.annotation.SuppressLint
import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.graphics.Color
import android.graphics.PixelFormat
import android.graphics.drawable.GradientDrawable
import android.os.Build
import android.os.IBinder
import android.util.TypedValue
import android.view.Gravity
import android.view.MotionEvent
import android.view.View
import android.view.WindowManager
import android.widget.Button
import android.widget.EditText
import android.widget.LinearLayout
import android.widget.TextView
import android.widget.Toast
import androidx.core.app.NotificationCompat
import com.flyhigao.mem.data.MemApiClient
import com.flyhigao.mem.data.PreferencesManager
import com.flyhigao.mem.ui.MainActivity
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

class FloatingInputService : Service() {

    private var windowManager: WindowManager? = null
    private var floatView: View? = null
    private val serviceScope = CoroutineScope(SupervisorJob() + Dispatchers.Main)
    private lateinit var prefs: PreferencesManager

    companion object {
        const val CHANNEL_ID = "mem_floating_channel"
        const val NOTIFICATION_ID = 1001
        const val ACTION_START = "ACTION_START"
        const val ACTION_STOP = "ACTION_STOP"
        var isRunning = false
            private set
    }

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onCreate() {
        super.onCreate()
        prefs = PreferencesManager(this)
        createNotificationChannel()
        startForeground(NOTIFICATION_ID, buildNotification())
        isRunning = true
        showFloatingWindow()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent?.action == ACTION_STOP) {
            stopSelf()
            return START_NOT_STICKY
        }
        return START_STICKY
    }

    private fun createNotificationChannel() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val channel = NotificationChannel(
                CHANNEL_ID,
                "Mem 悬浮窗服务",
                NotificationManager.IMPORTANCE_LOW
            ).apply {
                description = "保持 Mem 悬浮输入窗口在后台运行"
            }
            val manager = getSystemService(NotificationManager::class.java)
            manager.createNotificationChannel(channel)
        }
    }

    private fun buildNotification(): Notification {
        val openIntent = Intent(this, MainActivity::class.java)
        val pendingIntent = PendingIntent.getActivity(
            this, 0, openIntent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )

        val stopIntent = Intent(this, FloatingInputService::class.java).apply {
            action = ACTION_STOP
        }
        val stopPendingIntent = PendingIntent.getService(
            this, 1, stopIntent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )

        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setContentTitle("Mem 悬浮输入已就绪")
            .setContentText("点击打开主界面，或直接在悬浮窗中输入")
            .setSmallIcon(android.R.drawable.ic_menu_edit)
            .setContentIntent(pendingIntent)
            .addAction(android.R.drawable.ic_menu_close_clear_cancel, "关闭悬浮窗", stopPendingIntent)
            .setOngoing(true)
            .build()
    }

    @SuppressLint("ClickableViewAccessibility")
    private fun showFloatingWindow() {
        windowManager = getSystemService(Context.WINDOW_SERVICE) as WindowManager

        val dp = { value: Float ->
            TypedValue.applyDimension(TypedValue.COMPLEX_UNIT_DIP, value, resources.displayMetrics).toInt()
        }

        // Floating Card Layout (Pure code UI for ultra-lightweight & zero XML overhead)
        val rootLayout = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            val bg = GradientDrawable().apply {
                setColor(0xFF1E293B.toInt()) // slate-800
                cornerRadius = dp(14f).toFloat()
                setStroke(dp(1.5f), 0xFF38BDF8.toInt()) // primary border
            }
            background = bg
            setPadding(dp(12f), dp(10f), dp(12f), dp(12f))
            elevation = dp(8f).toFloat()
        }

        // Header (Drag bar)
        val headerLayout = LinearLayout(this).apply {
            orientation = LinearLayout.HORIZONTAL
            gravity = Gravity.CENTER_VERTICAL
            setPadding(0, 0, 0, dp(8f))
        }

        val titleView = TextView(this).apply {
            text = "⚡ Mem 快速中转"
            setTextColor(0xFF38BDF8.toInt())
            textSize = 13f
            typeface = android.graphics.Typeface.DEFAULT_BOLD
            layoutParams = LinearLayout.LayoutParams(0, LinearLayout.LayoutParams.WRAP_CONTENT, 1f)
        }

        val closeBtn = TextView(this).apply {
            text = "✕"
            setTextColor(0xFF94A3B8.toInt())
            textSize = 15f
            setPadding(dp(8f), dp(4f), dp(8f), dp(4f))
            setOnClickListener {
                stopSelf()
            }
        }

        headerLayout.addView(titleView)
        headerLayout.addView(closeBtn)
        rootLayout.addView(headerLayout)

        // Text Input
        val inputEdit = EditText(this).apply {
            hint = "在此输入要发送的文本..."
            setHintTextColor(0xFF64748B.toInt())
            setTextColor(Color.WHITE)
            textSize = 14f
            minLines = 3
            maxLines = 6
            gravity = Gravity.TOP or Gravity.START
            val editBg = GradientDrawable().apply {
                setColor(0xFF0F172A.toInt()) // slate-900
                cornerRadius = dp(8f).toFloat()
                setStroke(dp(1f), 0xFF334155.toInt())
            }
            background = editBg
            setPadding(dp(10f), dp(8f), dp(10f), dp(8f))
            layoutParams = LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT,
                LinearLayout.LayoutParams.WRAP_CONTENT
            )
        }
        rootLayout.addView(inputEdit)

        // Button row
        val btnRow = LinearLayout(this).apply {
            orientation = LinearLayout.HORIZONTAL
            gravity = Gravity.END or Gravity.CENTER_VERTICAL
            setPadding(0, dp(8f), 0, 0)
        }

        val pasteBtn = Button(this).apply {
            text = "📋 粘贴"
            textSize = 12f
            setTextColor(0xFFCBD5E1.toInt())
            val bgPaste = GradientDrawable().apply {
                setColor(0xFF334155.toInt())
                cornerRadius = dp(6f).toFloat()
            }
            background = bgPaste
            setPadding(dp(8f), dp(4f), dp(8f), dp(4f))
            layoutParams = LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.WRAP_CONTENT,
                dp(34f)
            ).apply { rightMargin = dp(8f) }
            setOnClickListener {
                val clipManager = getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
                val clip = clipManager.primaryClip
                if (clip != null && clip.itemCount > 0) {
                    val text = clip.getItemAt(0).text?.toString() ?: ""
                    inputEdit.append(text)
                } else {
                    Toast.makeText(this@FloatingInputService, "剪贴板为空", Toast.LENGTH_SHORT).show()
                }
            }
        }

        val sendBtn = Button(this).apply {
            text = "🚀 发送"
            textSize = 12f
            setTextColor(Color.WHITE)
            val bgSend = GradientDrawable().apply {
                setColor(0xFF0284C7.toInt()) // primary
                cornerRadius = dp(6f).toFloat()
            }
            background = bgSend
            setPadding(dp(12f), dp(4f), dp(12f), dp(4f))
            layoutParams = LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.WRAP_CONTENT,
                dp(34f)
            )
            setOnClickListener {
                val content = inputEdit.text.toString().trim()
                if (content.isEmpty()) {
                    Toast.makeText(this@FloatingInputService, "内容不能为空", Toast.LENGTH_SHORT).show()
                    return@setOnClickListener
                }

                isEnabled = false
                text = "发送中..."

                serviceScope.launch {
                    val serverUrl = prefs.getServerUrl()
                    val token = prefs.getApiToken()

                    if (token.isEmpty()) {
                        Toast.makeText(this@FloatingInputService, "请先在主界面设置 Token", Toast.LENGTH_LONG).show()
                        isEnabled = true
                        text = "🚀 发送"
                        return@launch
                    }

                    val result = MemApiClient.sendMessage(serverUrl, token, content, "Android")
                    if (result.isSuccess) {
                        Toast.makeText(this@FloatingInputService, "✅ 发送成功！Linux 已可读取", Toast.LENGTH_SHORT).show()
                        inputEdit.setText("")
                    } else {
                        val errMsg = result.exceptionOrNull()?.message ?: "未知错误"
                        Toast.makeText(this@FloatingInputService, "❌ 发送失败: $errMsg", Toast.LENGTH_LONG).show()
                    }

                    isEnabled = true
                    text = "🚀 发送"
                }
            }
        }

        btnRow.addView(pasteBtn)
        btnRow.addView(sendBtn)
        rootLayout.addView(btnRow)

        // Window Layout Params
        val layoutType = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            WindowManager.LayoutParams.TYPE_APPLICATION_OVERLAY
        } else {
            @Suppress("DEPRECATION")
            WindowManager.LayoutParams.TYPE_PHONE
        }

        val params = WindowManager.LayoutParams(
            dp(320f),
            WindowManager.LayoutParams.WRAP_CONTENT,
            layoutType,
            WindowManager.LayoutParams.FLAG_NOT_TOUCH_MODAL or WindowManager.LayoutParams.FLAG_WATCH_OUTSIDE_TOUCH,
            PixelFormat.TRANSLUCENT
        ).apply {
            gravity = Gravity.TOP or Gravity.CENTER_HORIZONTAL
            x = 0
            y = dp(80f)
        }

        // Dragging handler on header
        var initialX = 0
        var initialY = 0
        var initialTouchX = 0f
        var initialTouchY = 0f

        headerLayout.setOnTouchListener { _, event ->
            when (event.action) {
                MotionEvent.ACTION_DOWN -> {
                    initialX = params.x
                    initialY = params.y
                    initialTouchX = event.rawX
                    initialTouchY = event.rawY
                    true
                }
                MotionEvent.ACTION_MOVE -> {
                    params.x = initialX + (event.rawX - initialTouchX).toInt()
                    params.y = initialY + (event.rawY - initialTouchY).toInt()
                    windowManager?.updateViewLayout(rootLayout, params)
                    true
                }
                else -> false
            }
        }

        floatView = rootLayout
        windowManager?.addView(floatView, params)
    }

    override fun onDestroy() {
        super.onDestroy()
        isRunning = false
        serviceScope.cancel()
        if (floatView != null && windowManager != null) {
            try {
                windowManager?.removeView(floatView)
            } catch (e: Exception) {
                // Ignore if already removed
            }
            floatView = null
        }
    }
}
