/** @jsxImportSource solid-js */
import { createSignal, onCleanup } from "solid-js"
import { theme } from "../styles/theme"

type ToastType = "info" | "error" | "success"

export type ToastMessage = {
  id: number
  text: string
  type: ToastType
}

// Global toast state
let nextId = 0
const listeners: Set<(toast: ToastMessage) => void> = new Set()

export function showToast(text: string, type: ToastType = "info") {
  const toast: ToastMessage = { id: nextId++, text, type }
  listeners.forEach((fn) => fn(toast))
  // Auto-dismiss after 5 seconds
  setTimeout(() => {
    listeners.forEach((fn) => fn({ ...toast, text: "" }))
  }, 5000)
}

type ToastOverlayProps = {
  width: number
}

export function ToastOverlay(props: ToastOverlayProps) {
  const [toasts, setToasts] = createSignal<ToastMessage[]>([])

  const handler = (toast: ToastMessage) => {
    if (toast.text === "") {
      setToasts((prev) => prev.filter((t) => t.id !== toast.id))
    } else {
      setToasts((prev) => [...prev.slice(-4), toast])
    }
  }

  listeners.add(handler)
  onCleanup(() => listeners.delete(handler))

  return (
    <box
      width={props.width}
      height={toasts().length * 2}
      position="absolute"
      y={-toasts().length * 2 - 1}
    >
      {toasts().map((t) => (
        <box key={t.id} width="100%" height={1}>
          <text
            color={t.type === "error" ? theme.error : t.type === "success" ? theme.success : theme.textMuted}
          >
            {"  " + t.text}
          </text>
        </box>
      ))}
    </box>
  )
}
