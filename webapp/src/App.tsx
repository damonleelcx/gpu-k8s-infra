import { useState, useEffect, useCallback } from 'react'
import type { JobStatus, SubmitJobRequest } from './api'
import { listJobs, submitJob, getJobLogs, deleteJob } from './api'
import './App.css'

function App() {
  const [jobs, setJobs] = useState<JobStatus[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [logJob, setLogJob] = useState<string | null>(null)
  const [logs, setLogs] = useState<string>('')
  const [submitLoading, setSubmitLoading] = useState(false)
  const [submitError, setSubmitError] = useState<string | null>(null)

  const fetchJobs = useCallback(async () => {
    try {
      const data = await listJobs()
      setJobs(data)
      setError(null)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchJobs()
    const t = setInterval(fetchJobs, 5000)
    return () => clearInterval(t)
  }, [fetchJobs])

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    const form = e.currentTarget
    const spec: SubmitJobRequest = {
      name: (form.querySelector('[name="name"]') as HTMLInputElement)?.value || undefined,
      image: (form.querySelector('[name="image"]') as HTMLInputElement)?.value || 'nvidia/cuda:12.0.0-base-ubuntu22.04',
      gpuCount: parseInt((form.querySelector('[name="gpuCount"]') as HTMLInputElement)?.value || '1', 10),
      cpuRequest: (form.querySelector('[name="cpuRequest"]') as HTMLInputElement)?.value || '500m',
      memoryRequest: (form.querySelector('[name="memoryRequest"]') as HTMLInputElement)?.value || '2Gi',
    }
    const cmd = (form.querySelector('[name="command"]') as HTMLInputElement)?.value?.trim()
    if (cmd) spec.command = cmd.split(/\s+/).filter(Boolean)
    setSubmitLoading(true)
    setSubmitError(null)
    try {
      await submitJob(spec)
      form.reset()
      fetchJobs()
    } catch (e) {
      setSubmitError(e instanceof Error ? e.message : String(e))
    } finally {
      setSubmitLoading(false)
    }
  }

  const openLogs = async (jobName: string) => {
    setLogJob(jobName)
    setLogs('加载中...')
    try {
      const text = await getJobLogs(jobName)
      setLogs(text || '(无日志)')
    } catch (e) {
      setLogs('错误: ' + (e instanceof Error ? e.message : String(e)))
    }
  }

  const handleDelete = async (jobName: string) => {
    if (!confirm(`确定删除任务 ${jobName}？`)) return
    try {
      await deleteJob(jobName)
      if (logJob === jobName) setLogJob(null)
      fetchJobs()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    }
  }

  const phaseColor = (phase: string) => {
    switch (phase) {
      case 'Succeeded': return 'var(--success)'
      case 'Failed': return 'var(--error)'
      case 'Running': return 'var(--accent)'
      default: return 'var(--text-muted)'
    }
  }

  return (
    <div className="app">
      <header className="header">
        <h1>GPU 任务平台</h1>
        <p className="subtitle">提交与管理 Kubernetes GPU Job · 集成日志与指标</p>
      </header>

      <main className="main">
        <section className="card submit-card">
          <h2>提交 GPU 任务</h2>
          <form onSubmit={handleSubmit} className="form">
            <div className="form-row">
              <label>任务名（可选）</label>
              <input name="name" type="text" placeholder="gpu-job-1" className="input" />
            </div>
            <div className="form-row">
              <label>镜像</label>
              <input name="image" type="text" defaultValue="nvidia/cuda:12.0.0-base-ubuntu22.04" className="input" required />
            </div>
            <div className="form-row">
              <label>启动命令（可选，空格分隔）</label>
              <input name="command" type="text" placeholder="nvidia-smi sleep 60" className="input" />
            </div>
            <div className="form-row inline">
              <div>
                <label>GPU 数量</label>
                <input name="gpuCount" type="number" min={1} max={8} defaultValue={1} className="input narrow" />
              </div>
              <div>
                <label>CPU 请求</label>
                <input name="cpuRequest" type="text" defaultValue="500m" className="input narrow" />
              </div>
              <div>
                <label>内存请求</label>
                <input name="memoryRequest" type="text" defaultValue="2Gi" className="input narrow" />
              </div>
            </div>
            {submitError && <p className="error">{submitError}</p>}
            <button type="submit" className="btn btn-primary" disabled={submitLoading}>
              {submitLoading ? '提交中…' : '提交任务'}
            </button>
          </form>
        </section>

        <section className="card">
          <div className="card-head">
            <h2>任务列表</h2>
            <button type="button" className="btn btn-ghost" onClick={() => fetchJobs()}>刷新</button>
          </div>
          {loading && <p className="muted">加载中…</p>}
          {error && <p className="error">{error}</p>}
          {!loading && !error && jobs.length === 0 && <p className="muted">暂无任务</p>}
          {!loading && jobs.length > 0 && (
            <div className="table-wrap">
              <table className="table">
                <thead>
                  <tr>
                    <th>任务名</th>
                    <th>状态</th>
                    <th>Active / Succeeded / Failed</th>
                    <th>开始时间</th>
                    <th>操作</th>
                  </tr>
                </thead>
                <tbody>
                  {jobs.map((j) => (
                    <tr key={j.name}>
                      <td className="mono">{j.name}</td>
                      <td><span className="phase" style={{ color: phaseColor(j.phase) }}>{j.phase}</span></td>
                      <td>{j.active} / {j.succeeded} / {j.failed}</td>
                      <td className="mono small">{j.startTime || '—'}</td>
                      <td>
                        <button type="button" className="btn btn-ghost small" onClick={() => openLogs(j.name)}>日志</button>
                        <button type="button" className="btn btn-ghost small danger" onClick={() => handleDelete(j.name)}>删除</button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>

        {logJob && (
          <section className="card log-card">
            <div className="card-head">
              <h2>日志: {logJob}</h2>
              <button type="button" className="btn btn-ghost" onClick={() => setLogJob(null)}>关闭</button>
            </div>
            <pre className="log-content">{logs}</pre>
          </section>
        )}
      </main>

      <footer className="footer">
        <a href="/metrics" target="_blank" rel="noopener noreferrer">Prometheus 指标</a>
        <span className="muted">· GPU K8s Infra</span>
      </footer>
    </div>
  )
}

export default App
