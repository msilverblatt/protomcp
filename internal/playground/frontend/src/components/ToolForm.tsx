import { useState, useEffect, useMemo } from 'react'
import type { Tool, JsonSchema, JsonSchemaProperty } from '../types'

interface ToolFormProps {
  tool: Tool
  onSubmit: (name: string, args: Record<string, unknown>) => void
  loading: boolean
}

/**
 * Resolve the effective properties and required fields for a schema.
 * If the schema uses oneOf (discriminated union), we need the user to pick
 * a variant first, then merge that variant's properties.
 */
function resolveSchema(schema: JsonSchema, selectedVariant: number | null): {
  properties: Record<string, JsonSchemaProperty>
  required: Set<string>
  discriminator: { key: string; values: string[] } | null
} {
  // If there's no oneOf, just return the flat properties
  if (!schema.oneOf || schema.oneOf.length === 0) {
    return {
      properties: schema.properties || {},
      required: new Set(schema.required || []),
      discriminator: null,
    }
  }

  // Find the discriminator field: the property that has a `const` in each oneOf variant
  // (standard JSON Schema discriminated union pattern)
  let discriminatorKey: string | null = null
  const discriminatorValues: string[] = []

  for (const variant of schema.oneOf) {
    if (!variant.properties) continue
    for (const [key, prop] of Object.entries(variant.properties)) {
      if (prop.const !== undefined) {
        if (!discriminatorKey) discriminatorKey = key
        if (key === discriminatorKey) {
          discriminatorValues.push(String(prop.const))
        }
        break
      }
    }
  }

  // If we couldn't detect a discriminator via const, check for an enum-based discriminator
  // in the top-level properties
  if (!discriminatorKey && schema.properties) {
    for (const [key, prop] of Object.entries(schema.properties)) {
      if (prop.enum && prop.enum.length > 0) {
        discriminatorKey = key
        discriminatorValues.push(...prop.enum.map(String))
        break
      }
    }
  }

  const discriminator = discriminatorKey
    ? { key: discriminatorKey, values: discriminatorValues }
    : null

  // Merge base properties with selected variant properties
  const baseProps = schema.properties || {}
  const baseRequired = new Set(schema.required || [])

  if (selectedVariant === null || selectedVariant >= schema.oneOf.length) {
    return { properties: baseProps, required: baseRequired, discriminator }
  }

  const variant = schema.oneOf[selectedVariant]
  const variantProps = variant.properties || {}
  const variantRequired = new Set(variant.required || [])

  return {
    properties: { ...baseProps, ...variantProps },
    required: new Set([...baseRequired, ...variantRequired]),
    discriminator,
  }
}

export default function ToolForm({ tool, onSubmit, loading }: ToolFormProps) {
  const [values, setValues] = useState<Record<string, unknown>>({})
  const [errors, setErrors] = useState<string[]>([])
  const [selectedVariant, setSelectedVariant] = useState<number | null>(null)

  // Reset form when tool changes
  useEffect(() => {
    setValues({})
    setErrors([])
    setSelectedVariant(null)
  }, [tool.name])

  const { properties, required, discriminator } = useMemo(
    () => resolveSchema(tool.inputSchema, selectedVariant),
    [tool.inputSchema, selectedVariant],
  )

  const handleVariantChange = (index: number) => {
    setSelectedVariant(index)
    // Set the discriminator value automatically
    if (discriminator) {
      setValues((prev) => ({ ...prev, [discriminator.key]: discriminator.values[index] }))
    }
  }

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    const missing = [...required].filter((k) => {
      // Skip discriminator — it's set automatically
      if (discriminator && k === discriminator.key) return false
      const v = values[k]
      return v === undefined || v === '' || v === null
    })
    if (missing.length > 0) {
      setErrors(missing.map((k) => `${k} is required`))
      return
    }
    setErrors([])
    onSubmit(tool.name, values)
  }

  const setValue = (key: string, value: unknown) => {
    setValues((prev) => ({ ...prev, [key]: value }))
  }

  const renderField = (key: string, prop: JsonSchemaProperty, isRequired: boolean) => {
    // Skip the discriminator field — it's rendered as the variant selector
    if (discriminator && key === discriminator.key) return null

    return (
      <div key={key}>
        <label className="block text-xs text-gray-400 mb-1">
          {key}
          {isRequired && <span className="text-red-400 ml-0.5">*</span>}
          {prop.description && (
            <span className="ml-2 text-gray-500">({prop.description})</span>
          )}
        </label>
        {prop.enum && prop.enum.length > 0 ? (
          <select
            value={String(values[key] ?? '')}
            onChange={(e) => setValue(key, e.target.value)}
            className="w-full px-2 py-1 text-sm bg-gray-700 border border-gray-600 rounded text-gray-200 focus:outline-none focus:border-blue-500"
          >
            <option value="">Select...</option>
            {prop.enum.map((v) => (
              <option key={String(v)} value={String(v)}>
                {String(v)}
              </option>
            ))}
          </select>
        ) : prop.type === 'boolean' ? (
          <input
            type="checkbox"
            checked={!!values[key]}
            onChange={(e) => setValue(key, e.target.checked)}
            className="accent-blue-500"
          />
        ) : prop.type === 'integer' || prop.type === 'number' ? (
          <input
            type="number"
            step={prop.type === 'integer' ? 1 : 'any'}
            value={(values[key] as string) ?? ''}
            onChange={(e) =>
              setValue(key, e.target.value === '' ? '' : Number(e.target.value))
            }
            className="w-full px-2 py-1 text-sm bg-gray-700 border border-gray-600 rounded text-gray-200 focus:outline-none focus:border-blue-500"
          />
        ) : (
          <input
            type="text"
            value={(values[key] as string) ?? ''}
            onChange={(e) => setValue(key, e.target.value)}
            className="w-full px-2 py-1 text-sm bg-gray-700 border border-gray-600 rounded text-gray-200 focus:outline-none focus:border-blue-500"
          />
        )}
      </div>
    )
  }

  return (
    <form onSubmit={handleSubmit} className="p-3 space-y-3">
      <h3 className="text-sm font-semibold text-gray-200">{tool.name}</h3>

      {/* Discriminated union variant selector */}
      {discriminator && (
        <div>
          <label className="block text-xs text-gray-400 mb-1">
            {discriminator.key}
            <span className="text-red-400 ml-0.5">*</span>
          </label>
          <select
            value={selectedVariant ?? ''}
            onChange={(e) => handleVariantChange(Number(e.target.value))}
            className="w-full px-2 py-1 text-sm bg-gray-700 border border-gray-600 rounded text-gray-200 focus:outline-none focus:border-blue-500"
          >
            <option value="">Select action...</option>
            {discriminator.values.map((v, i) => (
              <option key={v} value={i}>
                {v}
              </option>
            ))}
          </select>
        </div>
      )}

      {/* Render fields (only when variant is selected for oneOf schemas, or always for flat schemas) */}
      {(!discriminator || selectedVariant !== null) &&
        Object.entries(properties).map(([key, prop]) =>
          renderField(key, prop, required.has(key)),
        )}

      {errors.length > 0 && (
        <div className="text-xs text-red-400">{errors.join(', ')}</div>
      )}
      <button
        type="submit"
        disabled={loading}
        className="px-4 py-1.5 text-sm bg-blue-600 hover:bg-blue-500 rounded text-white disabled:opacity-50 cursor-pointer disabled:cursor-not-allowed"
      >
        {loading ? (
          <span className="flex items-center gap-2">
            <span className="w-3 h-3 border-2 border-white/30 border-t-white rounded-full animate-spin" />
            Calling...
          </span>
        ) : (
          'Call'
        )}
      </button>
    </form>
  )
}
