package com.crosstalk.translator.util

/** Injectable clock for expiry and reconnect scheduling. */
fun interface Clock {
    fun nowEpochMs(): Long
}

class SystemClock : Clock {
    override fun nowEpochMs(): Long = System.currentTimeMillis()
}
