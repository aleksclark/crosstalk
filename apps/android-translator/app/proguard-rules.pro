# CrossTalk Android Translator — keep rules for generated/serialization/WebRTC.
-keepattributes *Annotation*, InnerClasses, EnclosingMethod, Signature
-keepclassmembers class ** {
    @kotlinx.serialization.SerialName <fields>;
}
-keep,includedescriptorclasses class com.crosstalk.translator.generated.** { *; }
-keep class com.crosstalk.translator.generated.**$$serializer { *; }
-keepclassmembers class com.crosstalk.translator.generated.** {
    *** Companion;
}
-keep class kotlinx.serialization.** { *; }
-keep class org.webrtc.** { *; }
-dontwarn org.webrtc.**
-dontwarn okhttp3.**
-dontwarn okio.**
