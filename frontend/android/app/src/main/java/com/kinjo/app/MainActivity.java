package com.kinjo.app;

import android.os.Bundle;
import com.getcapacitor.BridgeActivity;
import com.shardev.capacitor.googleauth.GoogleAuthPlugin;

public class MainActivity extends BridgeActivity {
    @Override
    public void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        registerPlugin(GoogleAuthPlugin.class);
    }
}
