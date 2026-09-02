package com.alfirus.controlcenter;
import android.os.Bundle; import androidx.appcompat.app.AppCompatActivity; import android.widget.TextView;
public class MainActivity extends AppCompatActivity {
    @Override protected void onCreate(Bundle b){ super.onCreate(b); TextView tv=new TextView(this); tv.setText("Control Center — Android\nB&W Minimal • API: "+ApiClient.BASE); tv.setPadding(48,48,48,48); setContentView(tv); }
}
