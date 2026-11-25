package com.kinjo.app

import android.net.Uri
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.PickVisualMediaRequest
import androidx.activity.result.contract.ActivityResultContracts.PickVisualMedia
import androidx.activity.result.contract.ActivityResultContracts.TakePicture
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.selection.toggleable
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.sp
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.BasicText
import androidx.compose.foundation.verticalScroll
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.painter.BitmapPainter
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.core.content.FileProvider
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.io.File

@Composable
fun ProfileScreen(onDone: () -> Unit) {
    val context = LocalContext.current
    val saved = remember { ProfileStore.load(context) }
    var name by rememberSaveable { mutableStateOf(saved?.name.orEmpty()) }
    var role by rememberSaveable { mutableStateOf(saved?.role.orEmpty()) }
    var looking by rememberSaveable { mutableStateOf(saved?.looking.orEmpty()) }
    var photo by remember { mutableStateOf(ProfileStore.photo(context)) }
    var photoError by remember { mutableStateOf<String?>(null) }
    var saveError by remember { mutableStateOf<String?>(null) }
    var saving by remember { mutableStateOf(false) }
    var agreed by rememberSaveable { mutableStateOf(ProfileStore.agreed(context)) }
    val scope = rememberCoroutineScope()

    fun usePhoto(uri: Uri?) {
        if (uri == null) return
        scope.launch {
            val picked = withContext(Dispatchers.IO) { ProfileStore.importPhoto(context, uri) }
            photoError = if (picked == null) "That photo couldn’t be read. Try another one." else null
            if (picked != null) photo = picked
        }
    }

    val pick = rememberLauncherForActivityResult(PickVisualMedia()) { usePhoto(it) }
    val cameraUri = remember {
        FileProvider.getUriForFile(context, "${context.packageName}.files", File(context.cacheDir, "camera.jpg"))
    }
    val camera = rememberLauncherForActivityResult(TakePicture()) { taken -> if (taken) usePhoto(cameraUri) }

    val profile = Profile(name, role, looking)
    Column(Modifier.fillMaxSize().padding(horizontal = 22.dp)) {
        Column(Modifier.weight(1f).verticalScroll(rememberScrollState()).padding(top = 24.dp, bottom = 18.dp)) {
            BasicText("Create your profile.", style = Kinjo.title)
            BasicText("This is exactly what people will see.", style = Kinjo.sub, modifier = Modifier.padding(top = 14.dp))
            PersonCard(
                photo?.let { remember(it) { BitmapPainter(it) } }, name, role, looking,
                Modifier.padding(top = 30.dp, bottom = 12.dp).width(200.dp).align(Alignment.CenterHorizontally),
            )
            Row(
                Modifier.align(Alignment.CenterHorizontally),
                horizontalArrangement = Arrangement.spacedBy(16.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                KLink("Upload photo") { pick.launch(PickVisualMediaRequest(PickVisualMedia.ImageOnly)) }
                KLink("Take photo") {
                    runCatching { camera.launch(cameraUri) }.onFailure { photoError = "No camera app found. Upload a photo instead." }
                }
                BasicText("Portrait works best", style = Kinjo.small.copy(color = Kinjo.Muted))
            }
            photoError?.let {
                BasicText(it, style = Kinjo.small.copy(color = Kinjo.Danger), modifier = Modifier.align(Alignment.CenterHorizontally))
            }
            Spacer(Modifier.height(14.dp))
            Field("Name", name, { name = it })
            Spacer(Modifier.height(14.dp))
            Field("What you do", role, { role = it }, hint = "e.g. Building in healthcare", maxWords = ROLE_WORDS)
            Spacer(Modifier.height(14.dp))
            Field(
                "What you’re looking for", looking, { looking = it },
                hint = "e.g. A technical co-founder for my health startup", maxWords = LOOKING_WORDS, multiline = true,
            )
            Row(
                Modifier.padding(top = 22.dp).fillMaxWidth()
                    .toggleable(value = agreed, role = Role.Checkbox) { agreed = it },
                horizontalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                KCheck(agreed, Modifier.padding(top = 2.dp))
                BasicText("I’m 18 or older and I accept the Terms & Conditions and the Privacy Policy.", style = Kinjo.small.copy(color = Kinjo.Muted, fontSize = 13.5.sp))
            }
            Row(Modifier.padding(start = 32.dp), horizontalArrangement = Arrangement.spacedBy(16.dp)) {
                KLink("Terms & Conditions") { openUrl(context, TERMS_URL) }
                KLink("Privacy Policy") { openUrl(context, PRIVACY_URL) }
            }
        }
        saveError?.let {
            BasicText(it, style = Kinjo.small.copy(color = Kinjo.Danger), modifier = Modifier.padding(top = 10.dp))
        }
        KButton(
            if (saving) "Saving…" else "Continue",
            onClick = {
                saving = true
                saveError = null
                scope.launch {
                    saveError = tryServer {
                        Api.saveProfile(context, profile, checkNotNull(ProfileStore.photoJpeg(context)))
                        ProfileStore.save(context, profile)
                    }
                    saving = false
                    if (saveError == null) onDone()
                }
            },
            enabled = !saving && agreed && profile.isComplete(hasPhoto = photo != null),
            modifier = Modifier.padding(top = 14.dp, bottom = 16.dp),
        )
    }
}
