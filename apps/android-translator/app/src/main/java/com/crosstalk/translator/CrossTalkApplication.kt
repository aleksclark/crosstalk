package com.crosstalk.translator

import android.app.Application
import com.crosstalk.translator.app.AppContainer

class CrossTalkApplication : Application() {
    lateinit var container: AppContainer
        private set

    override fun onCreate() {
        super.onCreate()
        container = AppContainer(this)
    }
}
