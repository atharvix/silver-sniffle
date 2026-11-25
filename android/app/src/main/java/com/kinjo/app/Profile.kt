package com.kinjo.app

import android.content.Context
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.graphics.ImageDecoder
import android.net.Uri
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.asImageBitmap
import java.io.File

data class Profile(val name: String, val role: String, val looking: String)

const val ROLE_WORDS = 7
const val LOOKING_WORDS = 20
private const val MAX_PHOTO_PX = 1440

fun Profile.isComplete(hasPhoto: Boolean) =
    hasPhoto && name.isNotBlank() &&
        wordCount(role) in 1..ROLE_WORDS && wordCount(looking) in 1..LOOKING_WORDS

/** Local copy of the user's card. shortcut: device only, until the server stores profiles. */
object ProfileStore {
    private fun prefs(context: Context) = context.getSharedPreferences("profile", Context.MODE_PRIVATE)
    private fun photoFile(context: Context) = File(context.filesDir, "photo.jpg")

    fun load(context: Context): Profile? {
        val p = prefs(context)
        val name = p.getString("name", null) ?: return null
        return Profile(name, p.getString("role", "").orEmpty(), p.getString("looking", "").orEmpty())
    }

    fun save(context: Context, profile: Profile) {
        prefs(context).edit()
            .putString("name", profile.name.trim())
            .putString("role", profile.role.trim())
            .putString("looking", profile.looking.trim())
            .apply()
    }

    /** Removes the card from this phone (after the account is deleted). */
    fun clear(context: Context) {
        prefs(context).edit().clear().commit()
        photoFile(context).delete()
    }

    fun photoJpeg(context: Context): ByteArray? = photoFile(context).takeIf { it.exists() }?.readBytes()

    fun photo(context: Context): ImageBitmap? =
        photoFile(context).takeIf { it.exists() }?.let { BitmapFactory.decodeFile(it.path)?.asImageBitmap() }

    /**
     * Decodes a picked or captured image, shrinks it and stores it as the card photo.
     * Re-encoding drops EXIF, so a photo's GPS tag never leaves the phone. Returns null if the image can't be read.
     */
    fun importPhoto(context: Context, uri: Uri): ImageBitmap? = runCatching {
        val bitmap = ImageDecoder.decodeBitmap(ImageDecoder.createSource(context.contentResolver, uri)) { decoder, info, _ ->
            val longest = maxOf(info.size.width, info.size.height)
            if (longest > MAX_PHOTO_PX) decoder.setTargetSampleSize(longest / MAX_PHOTO_PX)
            decoder.allocator = ImageDecoder.ALLOCATOR_SOFTWARE // compress() needs a software bitmap
        }
        // Write beside and swap, so a failed write never corrupts the current photo.
        val tmp = File(context.filesDir, "photo.tmp")
        tmp.outputStream().use { check(bitmap.compress(Bitmap.CompressFormat.JPEG, 85, it)) }
        check(tmp.renameTo(photoFile(context)))
        bitmap.asImageBitmap()
    }.getOrNull()
}
