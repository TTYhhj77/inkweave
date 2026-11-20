-keep class gomobile.** { *; }
-keepclassmembers class * {
    @android.webkit.JavascriptInterface <methods>;
}
