import { Camera, CameraResultType, CameraSource } from '@capacitor/camera';
import { Capacitor } from '@capacitor/core';
import { compressImage } from './api';

export type PhotoPickerSource = 'camera' | 'photos';

/**
 * Universal photo capture helper supporting:
 * 1. Native Capacitor Camera sensor
 * 2. Native Capacitor Gallery photo library
 * 3. Web Camera (<input capture="user">)
 * 4. Web File Dialog (standard file explorer)
 */
export async function pickPhoto(source: PhotoPickerSource): Promise<string> {
  if (Capacitor.isNativePlatform()) {
    try {
      const capSource = source === 'camera' ? CameraSource.Camera : CameraSource.Photos;
      const photo = await Camera.getPhoto({
        quality: 95,
        // Must stay false on Android: allowEditing fires a
        // com.android.camera.action.CROP intent, and when no crop app claims it
        // Android shows an "Open with" chooser full of unrelated apps. Editing
        // is unsupported on Android anyway.
        allowEditing: false,
        resultType: CameraResultType.DataUrl,
        source: capSource,
        width: 2000,
        height: 2000,
        correctOrientation: true,
      });

      if (photo.dataUrl) {
        return photo.dataUrl;
      }
    } catch (err: any) {
      const msg = err?.message?.toLowerCase() || '';
      if (msg.includes('cancel') || msg.includes('no image picked') || msg.includes('user cancelled')) {
        throw new Error('cancelled');
      }
      console.warn('Native camera error, attempting web fallback:', err);
    }
  }

  // Web fallback
  return new Promise((resolve, reject) => {
    const input = document.createElement('input');
    input.type = 'file';
    input.accept = 'image/*';

    if (source === 'camera') {
      input.setAttribute('capture', 'user');
    }

    input.onchange = async () => {
      const file = input.files?.[0];
      if (!file) {
        reject(new Error('cancelled'));
        return;
      }

      try {
        const compressed = await compressImage(file);
        resolve(compressed);
      } catch (e) {
        reject(e);
      }
    };

    input.oncancel = () => {
      reject(new Error('cancelled'));
    };

    input.click();
  });
}
