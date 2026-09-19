package com.flyhigao.mem.data

import com.google.gson.annotations.SerializedName

data class Message(
    val id: Long,
    @SerializedName("user_id") val userId: Long,
    @SerializedName("token_id") val tokenId: Long? = null,
    @SerializedName("token_name") val tokenName: String? = null,
    val content: String,
    val source: String? = "Android",
    @SerializedName("created_at") val createdAt: String
)

data class FileRecord(
    val id: Long,
    @SerializedName("user_id") val userId: Long,
    @SerializedName("token_id") val tokenId: Long? = null,
    @SerializedName("token_name") val tokenName: String? = null,
    val filename: String,
    @SerializedName("file_size") val fileSize: Long,
    @SerializedName("content_type") val contentType: String? = null,
    @SerializedName("created_at") val createdAt: String
)

data class FileListResponse(
    val files: List<FileRecord>,
    @SerializedName("total_size") val totalSize: Long,
    @SerializedName("max_size") val maxSize: Long
)

data class ApiResponse<T>(
    val success: Boolean,
    val data: T? = null,
    val error: String? = null
)

data class PostMessageRequest(
    val content: String,
    val source: String = "Android"
)
