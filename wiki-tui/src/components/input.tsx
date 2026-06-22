/** @jsxImportSource solid-js */
type PromptInputProps = {
  value: () => string
  onChange: (val: string) => void
  onSubmit: (val: string) => void
  placeholder?: string
  width: number
}

export function PromptInput(props: PromptInputProps) {
  function handleContentChange(value: string) {
    props.onChange(value)
  }

  function handleSubmit() {
    const val = props.value().trim()
    if (val) {
      props.onSubmit(val)
      props.onChange("")
    }
  }

  return (
    <textarea
      value={props.value()}
      onContentChange={handleContentChange}
      onSubmit={handleSubmit}
      placeholder={props.placeholder || ""}
      width={props.width}
      height={3}
      backgroundColor="#0d1117"
      color="#e0e0e0"
    />
  )
}
