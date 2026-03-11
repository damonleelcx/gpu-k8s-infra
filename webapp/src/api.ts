const API_BASE = ''

export type JobStatus = {
  name: string
  namespace: string
  phase: string
  ready: number
  active: number
  succeeded: number
  failed: number
  startTime?: string
  message?: string
}

export type JobListResponse = {
  jobs: JobStatus[]
}

export type SubmitJobRequest = {
  name?: string
  image: string
  command?: string[]
  args?: string[]
  gpuCount: number
  cpuRequest?: string
  memoryRequest?: string
  ttlSecondsAfterFinished?: number
}

export type SubmitJobResponse = {
  jobName: string
  taskId: string
  status: string
}

export async function listJobs(): Promise<JobStatus[]> {
  const r = await fetch(`${API_BASE}/api/v1/jobs`)
  if (!r.ok) throw new Error(await r.text())
  const data: JobListResponse = await r.json()
  return data.jobs
}

export async function submitJob(spec: SubmitJobRequest): Promise<SubmitJobResponse> {
  const body: Record<string, unknown> = {
    image: spec.image,
    gpuCount: spec.gpuCount ?? 1,
    cpuRequest: spec.cpuRequest ?? '500m',
    memoryRequest: spec.memoryRequest ?? '2Gi',
  }
  if (spec.name) body.name = spec.name
  if (spec.command?.length) body.command = spec.command
  if (spec.args?.length) body.args = spec.args
  if (spec.ttlSecondsAfterFinished != null) body.ttlSecondsAfterFinished = spec.ttlSecondsAfterFinished

  const r = await fetch(`${API_BASE}/api/v1/jobs`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!r.ok) throw new Error(await r.text())
  return r.json()
}

export async function getJobLogs(jobName: string): Promise<string> {
  const r = await fetch(`${API_BASE}/api/v1/jobs/${encodeURIComponent(jobName)}/logs`)
  if (!r.ok) throw new Error(await r.text())
  return r.text()
}

export async function deleteJob(jobName: string): Promise<void> {
  const r = await fetch(`${API_BASE}/api/v1/jobs/${encodeURIComponent(jobName)}`, { method: 'DELETE' })
  if (!r.ok) throw new Error(await r.text())
}
