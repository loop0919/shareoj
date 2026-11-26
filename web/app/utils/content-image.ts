import { errorCode } from './api-error'

const maxStoredBytes = 512 * 1024

// Keep transparent diagrams as PNG; photographs become JPEG. Never discard alpha.
export async function prepareContentImage(file: File): Promise<string> {
  if (!['image/png', 'image/jpeg', 'image/webp'].includes(file.type) || file.size > 10 * 1024 * 1024) {
    throw new Error('PNG・JPEG・WebPの画像を10MB以下で選んでください。')
  }
  const bitmap = await createImageBitmap(file)
  try {
    const canvas = document.createElement('canvas')
    const context = canvas.getContext('2d')!
    const scale = Math.min(1, 2048 / Math.max(bitmap.width, bitmap.height))
    canvas.width = Math.max(1, Math.round(bitmap.width * scale))
    canvas.height = Math.max(1, Math.round(bitmap.height * scale))
    context.drawImage(bitmap, 0, 0, canvas.width, canvas.height)
    const pixels = context.getImageData(0, 0, canvas.width, canvas.height).data
    let transparent = false
    for (let i = 3; i < pixels.length; i += 4) {
      if (pixels[i] !== 255) { transparent = true; break }
    }
    for (let attempt = 0; attempt < 8; attempt++) {
      // Leave room for the server's metadata-free re-encoding.
      const png = canvas.toDataURL('image/png')
      const jpeg = transparent ? png : canvas.toDataURL('image/jpeg', 0.85)
      const encoded = png.length <= jpeg.length ? png : jpeg
      const data = encoded.split(',')[1]!
      if (data.length * 3 / 4 <= maxStoredBytes * 0.8) return data
      canvas.width = Math.max(1, Math.round(canvas.width * 0.75))
      canvas.height = Math.max(1, Math.round(canvas.height * 0.75))
      context.drawImage(bitmap, 0, 0, canvas.width, canvas.height)
    }
    throw new Error('画像を十分に縮小できませんでした。小さい画像を選んでください。')
  } finally { bitmap.close() }
}

export function contentImageError(error: unknown): string {
  const code = errorCode(error)
  if (code === 'image_in_use') return '保存済みの本文や公開中の内容で使用されています。画像の参照を削除して保存・公開を更新してください。'
  if (code === 'invalid_image') return '画像を保存できませんでした。小さいPNG・JPEG・WebP画像でお試しください。'
  if (error instanceof Error && !('statusCode' in error) && !('response' in error)) return error.message
  return '画像の操作に失敗しました。ログイン状態や通信を確認し、再度お試しください。'
}
