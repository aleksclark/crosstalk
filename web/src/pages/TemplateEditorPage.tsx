import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { getTemplate, createTemplate, updateTemplate } from '@/lib/api/client'
import type { Role, Mapping, SessionTemplateCreate } from '@/lib/api/types'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card'
import { Select } from '@/components/ui/select'

interface MappingFormState {
  sourceRole: string
  sourceChannel: string
  sinkType: 'role' | 'record' | 'broadcast'
  sinkRole: string
  sinkChannel: string
}

function emptyMappingForm(): MappingFormState {
  return { sourceRole: '', sourceChannel: '', sinkType: 'role', sinkRole: '', sinkChannel: '' }
}

function mappingToForm(m: Mapping): MappingFormState {
  const [sourceRole = '', sourceChannel = ''] = (m.source ?? '').split(':')
  const sink = m.sink ?? ''
  if (sink === 'record') {
    return { sourceRole, sourceChannel, sinkType: 'record', sinkRole: '', sinkChannel: '' }
  }
  if (sink === 'broadcast') {
    return { sourceRole, sourceChannel, sinkType: 'broadcast', sinkRole: '', sinkChannel: '' }
  }
  const [sinkRole = '', sinkChannel = ''] = sink.split(':')
  return { sourceRole, sourceChannel, sinkType: 'role', sinkRole, sinkChannel }
}

function formToMapping(f: MappingFormState): Mapping {
  const source = f.sourceRole && f.sourceChannel ? `${f.sourceRole}:${f.sourceChannel}` : ''
  let sink = ''
  if (f.sinkType === 'record') {
    sink = 'record'
  } else if (f.sinkType === 'broadcast') {
    sink = 'broadcast'
  } else if (f.sinkRole && f.sinkChannel) {
    sink = `${f.sinkRole}:${f.sinkChannel}`
  }
  return { source, sink }
}

function validateTemplate(roles: Role[], mappingForms: MappingFormState[]): string[] {
  const errors: string[] = []
  const roleNames = new Set(roles.map((r) => r.name))
  const multiClientRoles = new Set(roles.filter((r) => r.multi_client).map((r) => r.name))

  for (const f of mappingForms) {
    if (f.sourceRole && !roleNames.has(f.sourceRole)) {
      errors.push(`Mapping source role "${f.sourceRole}" does not exist`)
    }
    if (f.sinkType === 'role' && f.sinkRole && !roleNames.has(f.sinkRole)) {
      errors.push(`Mapping target role "${f.sinkRole}" does not exist`)
    }
    if (f.sourceRole && multiClientRoles.has(f.sourceRole)) {
      errors.push(`Multi-client role "${f.sourceRole}" cannot be a mapping source`)
    }
  }

  return errors
}

export function TemplateEditorPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const isNew = id === 'new'

  const [name, setName] = useState('')
  const [isDefault, setIsDefault] = useState(false)
  const [roles, setRoles] = useState<Role[]>([{ name: '', multi_client: false }])
  const [mappingForms, setMappingForms] = useState<MappingFormState[]>([emptyMappingForm()])
  const [errors, setErrors] = useState<string[]>([])
  const [loading, setLoading] = useState(!isNew)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (!isNew && id) {
      void getTemplate(id)
        .then((t) => {
          setName(t.name)
          setIsDefault(t.is_default)
          setRoles(t.roles.length > 0 ? t.roles : [{ name: '', multi_client: false }])
          const forms = (t.mappings ?? []).map(mappingToForm)
          setMappingForms(forms.length > 0 ? forms : [emptyMappingForm()])
        })
        .catch(() => navigate('/templates'))
        .finally(() => setLoading(false))
    }
  }, [id, isNew, navigate])

  const handleAddRole = () => setRoles([...roles, { name: '', multi_client: false }])
  const handleRemoveRole = (index: number) => setRoles(roles.filter((_, i) => i !== index))
  const handleRoleChange = (index: number, field: keyof Role, value: string | boolean) => {
    setRoles(roles.map((r, i) => (i === index ? { ...r, [field]: value } : r)))
  }

  const handleAddMapping = () => setMappingForms([...mappingForms, emptyMappingForm()])
  const handleRemoveMapping = (index: number) => setMappingForms(mappingForms.filter((_, i) => i !== index))
  const handleMappingChange = (index: number, field: keyof MappingFormState, value: string) => {
    setMappingForms(mappingForms.map((m, i) => (i === index ? { ...m, [field]: value } : m)))
  }

  const handleSave = async () => {
    const validRoles = roles.filter((r) => r.name?.trim())
    const validForms = mappingForms.filter((f) => f.sourceRole && f.sourceChannel)
    const validationErrors = validateTemplate(validRoles, validForms)

    if (validationErrors.length > 0) {
      setErrors(validationErrors)
      return
    }

    setErrors([])
    setSaving(true)

    const data: SessionTemplateCreate = {
      name,
      is_default: isDefault,
      roles: validRoles,
      mappings: validForms.map(formToMapping),
    }

    try {
      if (isNew) {
        await createTemplate(data)
      } else if (id) {
        await updateTemplate(id, data)
      }
      navigate('/templates')
    } catch (err) {
      setErrors([err instanceof Error ? err.message : 'Failed to save'])
    } finally {
      setSaving(false)
    }
  }

  if (loading) return <div className="text-muted-foreground">Loading...</div>

  const roleNames = roles.filter((r) => r.name?.trim()).map((r) => r.name!)

  return (
    <div className="space-y-6 max-w-4xl">
      <div className="flex items-center justify-between">
        <h1 className="text-3xl font-bold text-foreground">
          {isNew ? 'Create Template' : 'Edit Template'}
        </h1>
        <div className="flex gap-2">
          <Button variant="outline" onClick={() => navigate('/templates')}>
            Cancel
          </Button>
          <Button onClick={handleSave} disabled={saving || !name.trim()} data-testid="save-template-button">
            {saving ? 'Saving...' : 'Save'}
          </Button>
        </div>
      </div>

      {errors.length > 0 && (
        <div className="text-sm text-destructive bg-destructive/10 p-3 rounded-md space-y-1" role="alert" data-testid="template-errors">
          {errors.map((e, i) => (
            <div key={i}>{e}</div>
          ))}
        </div>
      )}

      <Card>
        <CardHeader>
          <CardTitle>General</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="template-name">Name</Label>
            <Input
              id="template-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g. Translation"
              data-testid="template-name-input"
            />
          </div>
          <div className="flex items-center gap-2">
            <input
              type="checkbox"
              id="template-default"
              checked={isDefault}
              onChange={(e) => setIsDefault(e.target.checked)}
              className="rounded"
              data-testid="template-default-toggle"
            />
            <Label htmlFor="template-default">Default template</Label>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Roles</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          {roles.map((role, i) => (
            <div key={i} className="flex items-center gap-3" data-testid="role-row">
              <Input
                value={role.name}
                onChange={(e) => handleRoleChange(i, 'name', e.target.value)}
                placeholder="Role name"
                className="flex-1"
                data-testid="role-name-input"
              />
              <label className="flex items-center gap-1.5 text-sm text-muted-foreground whitespace-nowrap">
                <input
                  type="checkbox"
                  checked={role.multi_client}
                  onChange={(e) => handleRoleChange(i, 'multi_client', e.target.checked)}
                  data-testid="role-multi-client-toggle"
                />
                Multi-client
              </label>
              <Button variant="ghost" size="sm" onClick={() => handleRemoveRole(i)} disabled={roles.length <= 1}>
                Remove
              </Button>
            </div>
          ))}
          <Button variant="outline" size="sm" onClick={handleAddRole} data-testid="add-role-button">
            Add Role
          </Button>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Mappings</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          {mappingForms.map((mapping, i) => (
            <div key={i} className="flex items-center gap-2 flex-wrap" data-testid="mapping-row">
              <Select
                value={mapping.sourceRole}
                onChange={(e) => handleMappingChange(i, 'sourceRole', e.target.value)}
                className="w-32"
                data-testid="mapping-from-role"
              >
                <option value="">Source role</option>
                {roleNames.map((r) => (
                  <option key={r} value={r}>{r}</option>
                ))}
              </Select>
              <span className="text-muted-foreground">:</span>
              <Input
                value={mapping.sourceChannel}
                onChange={(e) => handleMappingChange(i, 'sourceChannel', e.target.value)}
                placeholder="channel"
                className="w-28"
                data-testid="mapping-from-channel"
              />
              <span className="text-muted-foreground">→</span>
              <Select
                value={mapping.sinkType}
                onChange={(e) => handleMappingChange(i, 'sinkType', e.target.value)}
                className="w-32"
                data-testid="mapping-to-type"
              >
                <option value="role">Role</option>
                <option value="record">Record</option>
                <option value="broadcast">Broadcast</option>
              </Select>
              {mapping.sinkType === 'role' && (
                <>
                  <Select
                    value={mapping.sinkRole}
                    onChange={(e) => handleMappingChange(i, 'sinkRole', e.target.value)}
                    className="w-32"
                    data-testid="mapping-to-role"
                  >
                    <option value="">Target role</option>
                    {roleNames.map((r) => (
                      <option key={r} value={r}>{r}</option>
                    ))}
                  </Select>
                  <span className="text-muted-foreground">:</span>
                  <Input
                    value={mapping.sinkChannel}
                    onChange={(e) => handleMappingChange(i, 'sinkChannel', e.target.value)}
                    placeholder="channel"
                    className="w-28"
                    data-testid="mapping-to-channel"
                  />
                </>
              )}
              <Button variant="ghost" size="sm" onClick={() => handleRemoveMapping(i)} disabled={mappingForms.length <= 1}>
                Remove
              </Button>
            </div>
          ))}
          <Button variant="outline" size="sm" onClick={handleAddMapping} data-testid="add-mapping-button">
            Add Mapping
          </Button>
        </CardContent>
      </Card>
    </div>
  )
}
