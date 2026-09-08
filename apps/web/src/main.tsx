import React, { useCallback, useEffect, useRef, useState } from 'react';
import { createRoot } from 'react-dom/client';
import { Activity, ArrowDownToLine, ArrowRight, Box, Check, ChevronDown, ChevronRight, Circle, Clock3, Code2, ExternalLink, Folder, GitBranch, Layers3, LoaderCircle, LogOut, Plus, Radio, Rocket, Search, Server, ShieldCheck, Terminal, X } from 'lucide-react';
import './styles.css';
import { OwnerGate } from './owner';
import { OperationsPanel } from './operations';
import { SourcePanel, GitHubSettings } from './source';
import { NodeStatus } from './node-status';
import { VariablesPanel, SettingsPanel } from './service-settings';

type Project = { id: string; name: string; createdAt: string };
type Service = { url:string;settings:{kind:string;memoryMB:number;cpuMillis:number;mountPath:string;volumeName:string;network:string};id: string; projectId: string; name: string; environment: string; host: string; activeId: string; desiredState: string; createdAt: string };
type Deployment = { id: string; serviceId: string; image: string; port: number; healthPath: string; status: string; error: string; logs: string; createdAt: string; updatedAt: string };
type Event = { id: number; deploymentId: string; stage: string; message: string; createdAt: string };
type State = { environments: {projectId:string;name:string}[];actions:{id:string;serviceId:string;kind:string;status:string;error:string}[]; projects: Project[]; services: Service[]; deployments: Deployment[]; events: Event[] };
type Modal = 'project' | 'environment' | 'service' | 'deploy' | null;
const empty: State = { environments:[],actions:[], projects: [], services: [], deployments: [], events: [] };
const terminal = ['active', 'failed', 'superseded'];
const pretty: Record<string, string> = { queued: 'Queued', pulling: 'Pulling image', starting: 'Starting', checking: 'Checking readiness', routing: 'Switching traffic', active: 'Active', failed: 'Failed', superseded: 'Replaced', stopped: 'Stopped', empty: 'Not deployed' };
const timestamp = (s: string) => new Date(s).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
const imageName = (s: string) => s.split('@')[0];

function Brand() { return <div className="brand"><span className="brand-icon"><Layers3 size={21} /></span>cloudrail<span className="alpha">α</span></div>; }
function Status({ value }: { value: string }) { return <span className={`status ${value}`}>{['queued','pulling','starting','checking','routing'].includes(value) ? <LoaderCircle className="spin" size={12} /> : <span className="status-dot" />}{pretty[value] || value}</span>; }

function Dialog({ title, description, close, children }: { title: string; description: string; close: () => void; children: React.ReactNode }) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    ref.current?.querySelector<HTMLInputElement>('input')?.focus();
    const key = (event: KeyboardEvent) => {
      if (event.key === 'Escape') close();
      if (event.key === 'Tab') {
        const els = [...(ref.current?.querySelectorAll<HTMLElement>('button:not(:disabled),input:not(:disabled),select:not(:disabled),a[href]') || [])];
        const first = els[0], last = els.at(-1);
        if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus(); }
        if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus(); }
      }
    };
    document.addEventListener('keydown', key);
    return () => { document.removeEventListener('keydown', key); previous?.focus(); };
  }, [close]);
  return <div className="modal-shade" onMouseDown={e => { if (e.target === e.currentTarget) close(); }}><div className="modal" ref={ref} role="dialog" aria-modal="true" aria-labelledby="dialog-title"><button className="icon-button close-modal" aria-label="Close dialog" onClick={close}><X size={18} /></button><div className="modal-icon"><Rocket size={22} /></div><h2 id="dialog-title">{title}</h2><p className="muted modal-description">{description}</p>{children}</div></div>;
}

function App({ onLogout }: { onLogout: () => void }) {
  const [data, setData] = useState<State>(empty);
  const [ready, setReady] = useState(false);
  const [error, setError] = useState('');
  const [projectID, setProjectID] = useState('');
  const [serviceID, setServiceID] = useState('');
 const [githubOpen,setGithubOpen]=useState(false);
 const [environment,setEnvironment]=useState('production');
  const [deploymentID, setDeploymentID] = useState('');
  const [tab, setTab] = useState<'deployments' | 'logs' | 'variables' | 'settings' | 'source'>('deployments');
  const [query, setQuery] = useState('');
  const [modal, setModal] = useState<Modal>(null);
  const [formError, setFormError] = useState('');
  const requestKey=useRef(crypto.randomUUID());
  const [busy, setBusy] = useState(false);
  const [name, setName] = useState('');
  const [image, setImage] = useState('');
  const [port, setPort] = useState('80');
  const [health, setHealth] = useState('/');
 const [serviceKind,setServiceKind]=useState('http');

  const request = useCallback(async <T,>(path: string, body?: unknown, method?: string): Promise<T> => {
    const response = await fetch(path, { method: method || (body === undefined ? 'GET' : 'POST'), credentials:'same-origin', headers: { 'X-Cloudrail-Request':'1', ...(path.endsWith('/deployments')&&body?{'Idempotency-Key':requestKey.current}:{}), 'Content-Type': 'application/json' }, body: body === undefined ? undefined : JSON.stringify(body), signal: AbortSignal.timeout(15000) });
    const result = await response.json();
    if (!response.ok) {
      if (response.status === 401) { onLogout(); setReady(false); setData(empty); }
      throw new Error(result.error || 'Request failed');
    }
    return result as T;
  }, [onLogout]);
  const refresh = useCallback(async () => { const next = await request<State>('/api/state'); setData(next); setReady(true); setError(''); return next; }, [request]);
  useEffect(() => {
    let done = false; let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      try { const next = await request<State>('/api/state'); if (!done) { setData(next); setReady(true); setError(''); } }
      catch (err) { if (!done) setError(err instanceof Error ? err.message : 'Connection lost'); }
      if (!done) timer = setTimeout(poll, 2000);
    };
    void poll(); return () => { done = true; clearTimeout(timer); };
  }, [request]);
  const project = data.projects.find(p => p.id === projectID) || data.projects[0];
  const services = data.services.filter(s => s.projectId === project?.id && s.environment===environment);
  const service = services.find(s => s.id === serviceID) || services[0];
  const deployments = data.deployments.filter(d => d.serviceId === service?.id);
  const selected = deployments.find(d => d.id === deploymentID) || deployments[0];
  const events = data.events.filter(e => e.deploymentId === selected?.id).sort((a, b) => a.id - b.id);
  const active = data.deployments.find(d => d.id === service?.activeId);
  const running = services.filter(s => s.activeId && s.desiredState!=='stopped').length;
  const pending = data.deployments.filter(d => services.some(s => s.id === d.serviceId) && !terminal.includes(d.status)).length;
  const close = useCallback(() => { setModal(null); setFormError(''); }, []);
  const open = (kind: Modal, previous?: Deployment) => {
    requestKey.current=crypto.randomUUID();setServiceKind('http');setName(''); setFormError(''); setImage(previous?.image || (service?.settings.kind==='postgres'?(active?.image||deployments[0]?.image||''):'')); setPort(String(previous?.port || (service?.settings.kind==='postgres'?5432:80))); setHealth(previous?.healthPath || '/'); setModal(kind);
  };
  const submit = async (event: React.FormEvent) => {
    event.preventDefault(); setBusy(true); setFormError('');
    try {
      if (modal === 'project') { const p = await request<Project>('/api/projects', { name }); setProjectID(p.id); setEnvironment('production'); setServiceID(''); }
      if (modal === 'environment' && project) { await request(`/api/projects/${project.id}/environments`,{name});setEnvironment(name);setServiceID(''); }
      if (modal === 'service' && project) { const s = await request<Service>(`/api/projects/${project.id}/${serviceKind==='postgres'?'databases':'services'}`, { name,environment }); setServiceID(s.id); setDeploymentID('');if(s.settings.kind==='postgres')setTab('settings'); }
      if (modal === 'deploy' && service) { const d = await request<Deployment>(`/api/services/${service.id}/deployments`, { image: image.trim(), port: Number(port), healthPath: health }); setDeploymentID(d.id); setTab('deployments'); }
      close(); await refresh();
    } catch (err) { setFormError(err instanceof Error ? err.message : 'Something went wrong'); }
    finally { setBusy(false); }
  };
  const signOut = async () => { await request('/auth/logout',{});onLogout(); };

  return <div className="app-shell">
    <aside className="sidebar"><Brand /><div className="workspace-label"><span className="avatar">P</span><div>Personal workspace<small>Self-hosted</small></div><span className="workspace-check"><Check size={13} /></span></div><div className="sidebar-section"><span>WORKSPACE</span></div><div className="nav-current"><Layers3 size={17} /> Projects <span className="count">{data.projects.length}</span></div><div className="sidebar-section projects-heading"><span>YOUR PROJECTS</span><button className="icon-button" aria-label="Create project" onClick={() => open('project')}><Plus size={15} /></button></div><div className="project-nav">{data.projects.map(p => <button key={p.id} className={p.id === project?.id ? 'selected' : ''} onClick={() => { setProjectID(p.id); setEnvironment('production'); setServiceID(''); setDeploymentID(''); setQuery(''); }}><Folder size={16} /><span>{p.name}</span>{p.id === project?.id && <ChevronRight size={14} />}</button>)}{!data.projects.length && <p className="sidebar-empty">Your next idea starts here.</p>}</div><div className="sidebar-bottom"><div className="node-card"><span className={`node-indicator ${error ? 'offline' : ''}`} /><div>Your workspace<small>{error ? 'Connection interrupted' : ready ? 'Control plane connected' : 'Connecting…'}</small></div><Server size={16} /></div><button className="signout" onClick={signOut}><LogOut size={15} /> Sign out<span>v0.1 alpha</span></button></div></aside>
    <div className="workspace">
      <header className="topbar"><div className="breadcrumbs"><span>Personal</span><ChevronRight size={13} /><strong>{project?.name || 'Projects'}</strong></div><div className="topbar-right"><button className="text-button" onClick={()=>setGithubOpen(true)}>GitHub</button><span className="local-tag"><span /> Self-hosted alpha</span><span className="avatar small">P</span><button className="icon-button mobile-signout" aria-label="Sign out" onClick={signOut}><LogOut size={14} /></button></div></header>
      {error && <div className="connection-error" role="alert">{error} · Retrying automatically. Displayed data may be out of date.</div>}
      {!ready ? <div className="loading-page"><LoaderCircle className="spin" /> Connecting to your workspace…</div> : !project ? <div className="welcome"><div className="welcome-illustration"><Layers3 size={45} /></div><div className="eyebrow">START SOMETHING GOOD</div><h1>Your ideas, ready for takeoff.</h1><p>Create a project to give your services a home.<br />Deploy on your own server, from one place.</p><button className="primary" onClick={() => open('project')}><Plus size={16} /> Create your first project</button><div className="welcome-steps"><span><Folder size={17} /> Create a project</span><ChevronRight size={15} /><span><Box size={17} /> Add a service</span><ChevronRight size={15} /><span><Rocket size={17} /> Deploy an image</span></div></div> : <>
      <div className="project-header"><div><div className="eyebrow">PROJECT OVERVIEW</div><h1>{project.name}</h1><p>A little less infrastructure. A little more building.</p></div><button className="primary" onClick={() => open('service')}><Plus size={16} /> New service</button></div>
      <div className="project-toolbar"><label className="environment environment-select"><GitBranch size={14} /><select aria-label="Environment" value={environment} onChange={e=>{setEnvironment(e.target.value);setServiceID('');setDeploymentID('')}}>{data.environments.filter(e=>e.projectId===project.id).map(e=><option key={e.name} value={e.name}>{e.name}</option>)}</select></label><button className="icon-button" aria-label="Create environment" onClick={()=>open('environment')}><Plus size={14}/></button><span className="toolbar-divider" /><div className="toolbar-stat"><span className="green-dot" />{running} running</div>{pending > 0 && <div className="toolbar-stat"><LoaderCircle size={12} className="spin" />{pending} deploying</div>}<div className="toolbar-spacer" /><NodeStatus/></div>
      <div className={`project-body ${service ? 'with-detail' : ''}`}>
        <section className="services-pane" aria-label="Services"><div className="pane-title"><h2>Services <span>{services.length}</span></h2><label className="search-box"><Search size={14} /><input value={query} onChange={e => setQuery(e.target.value)} placeholder="Filter services" aria-label="Filter services" /></label></div>
          {!services.length ? <div className="service-empty"><div className="empty-icon"><Box size={27} /></div><h3>Make room for your first service</h3><p>An API, a website, your next side project.<br />Start with a container image.</p><button className="secondary" onClick={() => open('service')}><Plus size={15} /> Add service</button></div> : <div className="service-grid">{services.filter(s => s.name.toLowerCase().includes(query.toLowerCase())).map(s => { const latest = data.deployments.find(d => d.serviceId === s.id); const status = latest && !terminal.includes(latest.status) ? latest.status : s.activeId ? (s.desiredState==='stopped'?'stopped':'active') : latest?.status || 'empty'; return <button key={s.id} className={`service-card ${s.id === service?.id ? 'selected' : ''}`} onClick={() => { setServiceID(s.id); setDeploymentID('');if(s.settings.kind==='postgres')setTab('settings'); }}><div className="service-card-top"><span className="service-icon"><Box size={21} /></span><ChevronRight size={16} /></div><h3>{s.name}</h3><div className="service-source"><Code2 size={13} /><span>{latest ? imageName(latest.image) : 'Container image'}</span></div><div className="service-card-bottom"><Status value={status} /><span>{latest?.status === 'failed' && s.activeId ? 'Latest deploy failed' : s.settings.kind==='postgres'?'Private database':'HTTP service'}</span></div></button>; })}{services.length > 0 && !services.some(s => s.name.toLowerCase().includes(query.toLowerCase())) && <p className="no-results">No services match “{query}”.</p>}<button className="add-service-card" onClick={() => open('service')}><Plus size={17} /> New service</button></div>}
          <div className="workspace-note"><ShieldCheck size={16} /><p><strong>Your infrastructure, in your hands.</strong><br />Applications run on your own Linux node.</p></div>
        </section>
        {service && <section className="detail-pane" aria-label={`${service.name} details`}><div className="detail-header"><span className="service-icon"><Box size={21} /></span><div><h2>{service.name}</h2><span className="muted">{environment} / {service.settings.kind==='postgres'?'PostgreSQL':'HTTP service'}</span></div><button className="primary compact" onClick={() => open('deploy')}><Rocket size={14} /> Deploy</button></div><div className="service-address">{service.settings.kind==='postgres'?<span>Private PostgreSQL · db-{service.id}:5432</span>:active && service.desiredState!=='stopped' ? <a href={service.url} target="_blank" rel="noreferrer"><span className="green-dot" />{service.url.replace(/^https?:\/\//,'')}<ExternalLink size={13} /></a> : <span><Circle size={11} /> {service.desiredState==='stopped'?'Service is stopped. Start it from Settings.':'Your URL will be ready after the first deployment'}</span>}</div><div className="detail-tabs" role="tablist" aria-label="Service views"><button role="tab" aria-selected={tab === 'deployments'} onClick={() => setTab('deployments')} className={tab === 'deployments' ? 'active' : ''}><Rocket size={14} /> Deployments <span>{deployments.length}</span></button><button role="tab" aria-selected={tab === 'logs'} onClick={() => setTab('logs')} className={tab === 'logs' ? 'active' : ''}><Terminal size={14} /> Runtime logs</button><button role="tab" aria-selected={tab==='source'} className={tab==='source'?'active':''} disabled={service.settings.kind==='postgres'} onClick={()=>setTab('source')}>Source</button><button role="tab" aria-selected={tab==='variables'} className={tab==='variables'?'active':''} disabled={service.settings.kind==='postgres'} onClick={()=>setTab('variables')}>Variables</button><button role="tab" aria-selected={tab==='settings'} className={tab==='settings'?'active':''} onClick={()=>setTab('settings')}>Settings</button></div>
          {tab==='source'?<SourcePanel key={service.id} serviceID={service.id} request={request}/>:tab==='variables'?<VariablesPanel key={service.id} serviceID={service.id} request={request}/>:tab==='settings'?<><SettingsPanel service={service} actions={data.actions.filter(a=>a.serviceId===service.id)} request={request} refresh={refresh}/><OperationsPanel key={service.id} service={service} services={data.services} request={request} refresh={refresh} pending={data.actions.some(a=>a.serviceId===service.id&&(a.status==='queued'||a.status==='running'))}/></>:!selected ? <div className="first-deploy"><div className="empty-icon"><Rocket size={28} /></div><h3>Ready when you are.</h3><p>Choose an image and we’ll take care<br />of starting it and routing traffic.</p><button className="primary" onClick={() => open('deploy')}>Deploy an image <ArrowRight size={15} /></button><span className="first-deploy-note"><ShieldCheck size={13} /> Readiness checked before going live</span></div> : <div className="deployment-content">{!terminal.includes(selected.status)&&<button className="secondary" onClick={async()=>{try{await request(`/api/deployments/${selected.id}/cancel`,{});await refresh()}catch(e){setError((e as Error).message)}}}>Cancel deployment</button>}<div className="history-selector"><label htmlFor="release">Deployment</label><select id="release" value={selected.id} onChange={e => setDeploymentID(e.target.value)}>{deployments.map(d => <option key={d.id} value={d.id}>{d.id.slice(0, 8)} · {pretty[d.status]} · {new Date(d.createdAt).toLocaleString()}</option>)}</select><ChevronDown size={13} /></div>
          {tab === 'deployments' ? <><div className={`release-card ${selected.status}`}><div className="release-top"><Status value={selected.status} /><span className="release-id">{selected.id.slice(0, 8)}</span></div><h3>{imageName(selected.image)}</h3><p className="digest" title={selected.image}>{selected.image.split('@')[1]}</p><div className="release-meta"><span><Clock3 size={12} />{new Date(selected.createdAt).toLocaleString()}</span><span>Port {selected.port}</span></div></div>{selected.error && <div className="failure-message" role="status"><strong>This deployment didn’t go live.</strong><p>{selected.error}</p>{active && <span><ShieldCheck size={13} /> Previous release is still selected for traffic.</span>}</div>}<div className="activity-heading"><h3><Activity size={14} /> Deployment activity</h3><span>Auto-refreshes</span></div><ol className="timeline">{events.map(e => <li key={e.id}><span className={`timeline-point ${e.stage}`}>{e.stage === 'failed' ? <X size={11} /> : <Check size={10} />}</span><div><strong>{pretty[e.stage]}</strong><p>{e.message}</p></div><time>{timestamp(e.createdAt)}</time></li>)}</ol>{terminal.includes(selected.status) && <button className="secondary redeploy" onClick={() => open('deploy', selected)}><ArrowDownToLine size={14} />{selected.status === 'superseded' ? 'Deploy this version again' : 'Redeploy image'}</button>}</> : <div className="logs-view"><div className="logs-toolbar"><span><Terminal size={13} /> stdout / stderr</span><span>Last 80 lines</span></div><pre>{selected.logs || 'No runtime logs captured for this deployment yet.'}</pre><p className="logs-note">Active release logs refresh while the deployment worker is idle. Avoid logging secrets.</p></div>}
          </div>}
        </section>}
      </div></>}
      <footer className="workspace-footer"><span><Radio size={12} /> {error ? 'Reconnecting' : 'Local development preview'}</span><span>Built to give you control.</span></footer>
    </div>
    {githubOpen&&<GitHubSettings request={request} close={()=>setGithubOpen(false)}/>}
    {modal && <Dialog title={modal === 'environment' ? 'Create environment' : modal === 'project' ? 'Create a project' : modal === 'service' ? 'Add a service' : 'Deploy an image'} description={modal === 'environment' ? 'A separate home for staging or another deployment configuration.' : modal === 'project' ? 'A home for the services that belong together.' : modal === 'service' ? 'Give your application a name. You can deploy an image next.' : service?.settings.mountPath ? 'The current container stops before the persistent replacement starts.' : 'We’ll check the new release before switching traffic.'} close={close}><form onSubmit={submit}>{modal !== 'deploy' ? <><label htmlFor="resource-name">{modal === 'project' ? 'Project' : modal==='environment'?'Environment':'Service'} name</label><input id="resource-name" value={name} onChange={e => setName(e.target.value)} maxLength={60} required pattern="[a-zA-Z0-9][a-zA-Z0-9 _.\-]*" placeholder={modal === 'project' ? 'My next great idea' : 'api-server'} />{modal === 'service' && <><label className="spaced-label" htmlFor="service-kind">Service type</label><select id="service-kind" value={serviceKind} onChange={e=>setServiceKind(e.target.value)}><option value="http">Application</option><option value="postgres">PostgreSQL database</option></select><p className="field-help">Added to {project?.name} / {environment}{serviceKind==='postgres'?' · Includes a persistent volume and private credentials.':''}</p></>}</> : <><label htmlFor="image">Container image</label><input id="image" readOnly={service?.settings.kind==='postgres'} value={image} onChange={e => setImage(e.target.value)} required maxLength={512} placeholder="repository@sha256:…" autoComplete="off" /><p className="field-help">Public image with an immutable SHA-256 digest. Tags alone aren’t supported yet.</p><div className="form-row"><div><label htmlFor="port">Container port</label><input id="port" readOnly={service?.settings.kind==='postgres'} type="number" min={1} max={65535} value={port} onChange={e => setPort(e.target.value)} required /></div><div><label htmlFor="health">Readiness path</label><input id="health" value={health} onChange={e => setHealth(e.target.value)} maxLength={200} required placeholder="/health" /></div></div><div className="form-note"><ShieldCheck size={16} /><span>Uses current service variables and resource settings. {service?.settings.mountPath ? 'Persistent services pause while replacing their container. Image rollback does not restore stored data.' : 'The current release stays online until the candidate is ready.'} Limits: {service?.settings.memoryMB ?? 256} MB RAM, {(service?.settings.cpuMillis ?? 1000) / 1000} CPU.</span></div></>}{formError && <p className="error-text" role="alert">{formError}</p>}<div className="modal-actions"><button type="button" className="secondary" onClick={close} disabled={busy}>Cancel</button><button className="primary" type="submit" disabled={busy}>{busy ? <LoaderCircle size={15} className="spin" /> : modal === 'deploy' ? <Rocket size={15} /> : <Plus size={15} />}{busy ? 'Saving…' : modal === 'project' ? 'Create project' : modal === 'environment'?'Create environment':modal === 'service' ? 'Create service' : 'Deploy image'}</button></div></form></Dialog>}
  </div>;
}

createRoot(document.getElementById('root')!).render(<React.StrictMode><OwnerGate>{logout=><App onLogout={logout}/>}</OwnerGate></React.StrictMode>);
