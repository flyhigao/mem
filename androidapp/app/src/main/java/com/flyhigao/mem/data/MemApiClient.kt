package com.flyhigao.mem.data

import com.google.gson.Gson
import com.google.gson.reflect.TypeToken
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import java.util.concurrent.TimeUnit

object MemApiClient {
    private val client = OkHttpClient.Builder()
        .connectTimeout(5, TimeUnit.SECONDS)
        .readTimeout(5, TimeUnit.SECONDS)
        .writeTimeout(5, TimeUnit.SECONDS)
        .build()

    private val gson = Gson()
    private val JSON_MEDIA = "application/json; charset=utf-8".toMediaType()

    suspend fun sendMessage(
        serverUrl: String,
        token: String,
        content: String,
        source: String = "Android"
    ): Result<Message> = withContext(Dispatchers.IO) {
        try {
            val url = "${serverUrl.trimEnd('/')}/api/v1/messages"
            val payload = gson.toJson(PostMessageRequest(content = content, source = source))
            val body = payload.toRequestBody(JSON_MEDIA)

            val request = Request.Builder()
                .url(url)
                .addHeader("Authorization", "Bearer $token")
                .post(body)
                .build()

            client.newCall(request).execute().use { response ->
                val respStr = response.body?.string() ?: ""
                if (!response.isSuccessful) {
                    return@withContext Result.failure(Exception("HTTP ${response.code}: $respStr"))
                }
                val type = object : TypeToken<ApiResponse<Message>>() {}.type
                val apiResp: ApiResponse<Message> = gson.fromJson(respStr, type)
                if (apiResp.success && apiResp.data != null) {
                    Result.success(apiResp.data)
                } else {
                    Result.failure(Exception(apiResp.error ?: "未知错误"))
                }
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    suspend fun getMessages(
        serverUrl: String,
        token: String,
        limit: Int = 50
    ): Result<List<Message>> = withContext(Dispatchers.IO) {
        try {
            val url = "${serverUrl.trimEnd('/')}/api/v1/messages?limit=$limit"
            val request = Request.Builder()
                .url(url)
                .addHeader("Authorization", "Bearer $token")
                .get()
                .build()

            client.newCall(request).execute().use { response ->
                val respStr = response.body?.string() ?: ""
                if (!response.isSuccessful) {
                    return@withContext Result.failure(Exception("HTTP ${response.code}: $respStr"))
                }
                val type = object : TypeToken<ApiResponse<List<Message>>>() {}.type
                val apiResp: ApiResponse<List<Message>> = gson.fromJson(respStr, type)
                if (apiResp.success) {
                    Result.success(apiResp.data ?: emptyList())
                } else {
                    Result.failure(Exception(apiResp.error ?: "未知错误"))
                }
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    suspend fun getLatestMessage(
        serverUrl: String,
        token: String
    ): Result<Message?> = withContext(Dispatchers.IO) {
        try {
            val url = "${serverUrl.trimEnd('/')}/api/v1/messages/latest"
            val request = Request.Builder()
                .url(url)
                .addHeader("Authorization", "Bearer $token")
                .get()
                .build()

            client.newCall(request).execute().use { response ->
                val respStr = response.body?.string() ?: ""
                if (!response.isSuccessful) {
                    return@withContext Result.failure(Exception("HTTP ${response.code}: $respStr"))
                }
                val type = object : TypeToken<ApiResponse<Message?>>() {}.type
                val apiResp: ApiResponse<Message?> = gson.fromJson(respStr, type)
                if (apiResp.success) {
                    Result.success(apiResp.data)
                } else {
                    Result.failure(Exception(apiResp.error ?: "未知错误"))
                }
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    suspend fun testConnection(serverUrl: String, token: String): Result<String> = withContext(Dispatchers.IO) {
        try {
            val url = "${serverUrl.trimEnd('/')}/api/v1/ping"
            val request = Request.Builder().url(url).get().build()
            client.newCall(request).execute().use { response ->
                if (!response.isSuccessful) {
                    return@withContext Result.failure(Exception("服务器连接失败 (HTTP ${response.code})"))
                }
            }

            // Test token
            val msgResult = getMessages(serverUrl, token, limit = 1)
            if (msgResult.isSuccess) {
                Result.success("连接成功！Token 鉴权通过")
            } else {
                Result.failure(Exception("服务器连接成功，但 Token 鉴权失败: ${msgResult.exceptionOrNull()?.message}"))
            }
        } catch (e: Exception) {
            Result.failure(Exception("无法连接到服务器: ${e.message}"))
        }
    }
}
