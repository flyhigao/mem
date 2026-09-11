package com.flyhigao.mem.data

import android.content.Context
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map

private val Context.dataStore by preferencesDataStore(name = "mem_settings")

class PreferencesManager(private val context: Context) {

    companion object {
        val KEY_SERVER_URL = stringPreferencesKey("server_url")
        val KEY_API_TOKEN = stringPreferencesKey("api_token")
        const val DEFAULT_SERVER_URL = "http://192.168.2.97:8080"
    }

    val serverUrlFlow: Flow<String> = context.dataStore.data.map { prefs ->
        prefs[KEY_SERVER_URL] ?: DEFAULT_SERVER_URL
    }

    val apiTokenFlow: Flow<String> = context.dataStore.data.map { prefs ->
        prefs[KEY_API_TOKEN] ?: ""
    }

    suspend fun getServerUrl(): String {
        return serverUrlFlow.first()
    }

    suspend fun getApiToken(): String {
        return apiTokenFlow.first()
    }

    suspend fun saveServerUrl(url: String) {
        context.dataStore.edit { prefs ->
            prefs[KEY_SERVER_URL] = url.trim().trimEnd('/')
        }
    }

    suspend fun saveApiToken(token: String) {
        context.dataStore.edit { prefs ->
            prefs[KEY_API_TOKEN] = token.trim()
        }
    }
}
