import React, { useCallback, useEffect, useRef, useState } from 'react';
import { createRoot } from 'react-dom/client';
import { Activity, ArrowDownToLine, ArrowRight, Box, Check, ChevronDown, ChevronRight, Circle, Clock3, Cloud, Database, ExternalLink, Folder, GitBranch, HardDrive, Layers3, LoaderCircle, LogOut, Plus, Radio, Rocket, Server, ShieldCheck, Terminal, X } from 'lucide-react';
import './styles.css';
import { OwnerGate } from './owner';
import { OperationsPanel } from './operations';
import { SourcePanel, GitHubSettings } from './source';
import { NodeStatus } from './node-status';
import { VariablesPanel, SettingsPanel } from './service-settings';
import { ProjectCanvas, type CanvasResource, type CreateIntent } from './project-canvas';

type Project = { id: string; name: string; createdAt: string };
type Volume = { id:string; projectId:string; environment:string; name:string };
type Bucket = { id:string; projectId:string; environment:string; name:string; provider:string; endpoint:string; region:string; bucketName:string; forcePathStyle:boolean; credentialVersion:number };
type RuntimeConfig={kind:string;memoryMB:number;cpuMillis:number;mountPath:string;volumeName:string;network:string;startCommand?:string;preDeployCommand?:string;preDeployTimeoutSeconds?:number;restartPolicy?:string;restartMaxRetries?:number};
type Service = { url:string;settings:RuntimeConfig;id: string; projectId: string; name: string; environment: string; host: string; activeId: string; desiredState: string; resourceKind:string; workloadMode:string; template:string; templateVersion?:string; cronSchedule:string; cronNextRun?:string; createdAt: string };
type Deployment = { id: string; serviceId: string; image: string; port: number; healthPath: string; status: string; error: string; logs: string; settings?:RuntimeConfig; createdAt: string; updatedAt: string };
type Event = { id: number; deploymentId: string; stage: string; message: string; createdAt: string };
type CronRun = {id:string;serviceId:string;deploymentId:string;scheduledFor:string;status:string;exitCode?:number;logs:string;error:string;startedAt?:string;finishedAt?:string};
type State = { environments: {projectId:string;name:string}[];actions:{id:string;serviceId:string;kind:string;status:string;error:string}[]; projects: Project[]; services: Service[]; deployments: Deployment[]; events: Event[]; cronRuns:CronRun[] };
type Modal = 'project' | 'environment' | 'service' | 'deploy' | null;
const createCopy: Record<CreateIntent,{title:string;description:string;type:string}> = {
  github:{title:'GitHub Repository',description:'Create the service, then choose its repository, branch and builder.',type:'Web / API service'},
  image:{title:'Docker Image',description:'Create the service, then deploy an immutable container image.',type:'Web / API service'},
  empty:{title:'Empty Service',description:'Create the service now and configure its source when you are ready.',type:'Web / API service'},
  worker:{title:'Background Worker',description:'Create a long-running process without a public HTTP route.',type:'Background worker'},
  cron:{title:'Cron Job',description:'Create a scheduled process. Its UTC schedule is configured before activation.',type:'Scheduled job'},
  postgres:{title:'PostgreSQL',description:'Create a private database with persistent storage and generated credentials.',type:'Database template'},
  redis:{title:'Redis',description:'Create a private Redis service with append-only persistence and generated credentials.',type:'Data template'},
  mysql:{title:'MySQL',description:'Create a private MySQL service with persistent InnoDB storage and generated credentials.',type:'Database template'},
  mongo:{title:'MongoDB',description:'Create a private MongoDB service with persistent document storage and generated credentials.',type:'Database template'},
  volume:{title:'Persistent Volume',description:'Create storage that can be attached to one application in this environment.',type:'Storage resource'},
  bucket:{title:'S3-compatible Bucket',description:'Connect an existing private S3-compatible bucket and keep its credentials encrypted.',type:'Object storage resource'},
};
const empty: State = { environments:[],actions:[], projects: [], services: [], deployments: [], events: [], cronRuns:[] };
const terminal = ['active', 'failed', 'superseded'];
const pretty: Record<string, string> = { queued: 'Queued', pulling: 'Pulling image', predeploy: 'Pre-deploy command', starting: 'Starting', checking: 'Checking readiness', routing: 'Switching traffic', active: 'Active', failed: 'Failed', superseded: 'Replaced', stopped: 'Stopped', empty: 'Not deployed' };
const timestamp = (s: string) => new Date(s).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
const imageName = (s: string) => s.split('@')[0];
const isDataService = (service?: Service) => service?.resourceKind === 'database';
const dataName = (service?: Service) => service?.template === 'redis' ? 'Redis' : service?.template === 'mysql' ? 'MySQL' : service?.template === 'mongo' ? 'MongoDB' : 'PostgreSQL';
const dataPort = (service?: Service) => service?.template === 'redis' ? 6379 : service?.template === 'mysql' ? 3306 : service?.template === 'mongo' ? 27017 : 5432;

function Brand() { return <div className="brand"><span className="brand-icon"><Layers3 size={21} /></span>cloudrail<span className="alpha">α</span></div>; }
function Status({ value }: { value: string }) { return <span className={`status ${value}`}>{['queued','pulling','predeploy','starting','checking','routing'].includes(value) ? <LoaderCircle className="spin" size={12} /> : <span className="status-dot" />}{pretty[value] || value}</span>; }

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
  const [selectedKey, setSelectedKey] = useState(() => new URLSearchParams(window.location.search).get('resource') || '');
  const [createOpen, setCreateOpen] = useState(false);
  const [createIntent, setCreateIntent] = useState<CreateIntent>('empty');
 const [githubOpen,setGithubOpen]=useState(false);
 const [environment,setEnvironment]=useState('production');
  const [deploymentID, setDeploymentID] = useState('');
  const [tab, setTab] = useState<'deployments' | 'logs' | 'variables' | 'settings' | 'source'>('deployments');
  const [modal, setModal] = useState<Modal>(null);
  const [formError, setFormError] = useState('');
  const requestKey=useRef(crypto.randomUUID());
  const [busy, setBusy] = useState(false);
  const [name, setName] = useState('');
  const [image, setImage] = useState('');
  const [port, setPort] = useState('80');
  const [health, setHealth] = useState('/');
 const [serviceKind,setServiceKind]=useState('http');
 const [sourceType,setSourceType]=useState<'github'|'image'|'empty'>('empty');
 const [workloadMode,setWorkloadMode]=useState<'web'|'worker'|'cron'>('web');
 const [bucketEndpoint,setBucketEndpoint]=useState('');
 const [bucketRegion,setBucketRegion]=useState('us-east-1');
 const [remoteBucket,setRemoteBucket]=useState('');
 const [bucketAccessKey,setBucketAccessKey]=useState('');
 const [bucketSecretKey,setBucketSecretKey]=useState('');
 const [bucketPathStyle,setBucketPathStyle]=useState(true);

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
  const service = services.find(s => s.id === serviceID);
  const deployments = data.deployments.filter(d => d.serviceId === service?.id);
  const selected = deployments.find(d => d.id === deploymentID) || deployments[0];
  const events = data.events.filter(e => e.deploymentId === selected?.id).sort((a, b) => a.id - b.id);
  const active = data.deployments.find(d => d.id === service?.activeId);
  const running = services.filter(s => s.activeId && s.desiredState!=='stopped').length;
  const pending = data.deployments.filter(d => services.some(s => s.id === d.serviceId) && !terminal.includes(d.status)).length;
  const close = useCallback(() => { setModal(null); setFormError(''); }, []);
  const open = (kind: Modal, previous?: Deployment) => {
    requestKey.current=crypto.randomUUID();setName(''); setFormError(''); setImage(previous?.image || (isDataService(service)?(active?.image||deployments[0]?.image||''):'')); setPort(String(previous?.port || (isDataService(service)?dataPort(service):80))); setHealth(previous?.healthPath || '/'); setModal(kind);
  };
  const setLocationResource = useCallback((key: string, push = true) => {
    setSelectedKey(key);
    const url = new URL(window.location.href);
    if (key) url.searchParams.set('resource', key); else url.searchParams.delete('resource');
    window.history[push ? 'pushState' : 'replaceState']({}, '', url);
  }, []);
  const closeSelection = useCallback(() => {
    setServiceID(''); setDeploymentID(''); setLocationResource('');
  }, [setLocationResource]);
  const selectResource = useCallback((resource: CanvasResource, navigate = true) => {
    setLocationResource(resource.key, navigate);
    if (resource.kind === 'service' || resource.kind === 'database') {
      setServiceID(resource.id); setDeploymentID(''); setTab(resource.kind === 'database' ? 'settings' : 'deployments');
    } else {
      setServiceID(''); setDeploymentID('');
    }
  }, [setLocationResource]);
  useEffect(() => {
    const restoreSelection = () => {
      const key = new URLSearchParams(window.location.search).get('resource') || '';
      setSelectedKey(key);
      setServiceID(key.startsWith('service:') ? key.slice('service:'.length) : '');
      setDeploymentID('');
    };
    window.addEventListener('popstate', restoreSelection);
    return () => window.removeEventListener('popstate', restoreSelection);
  }, []);
  const beginCreate = (intent: CreateIntent) => {
    setCreateIntent(intent); setCreateOpen(false); setServiceKind(intent === 'postgres' || intent === 'redis' || intent === 'mysql' || intent === 'mongo' || intent === 'volume' || intent === 'bucket' ? intent : 'http');
    setSourceType(intent === 'github' ? 'github' : intent === 'image' ? 'image' : 'empty');
    setWorkloadMode(intent === 'worker' ? 'worker' : intent === 'cron' ? 'cron' : 'web');
    setName(''); setBucketEndpoint(''); setBucketRegion('us-east-1'); setRemoteBucket(''); setBucketAccessKey(''); setBucketSecretKey(''); setBucketPathStyle(true); setFormError(''); setModal('service');
  };
  const submit = async (event: React.FormEvent) => {
    event.preventDefault(); setBusy(true); setFormError('');
    try {
      let openImageDeploy = false;
      if (modal === 'project') { const p = await request<Project>('/api/projects', { name }); setProjectID(p.id); setEnvironment('production'); closeSelection(); }
      if (modal === 'environment' && project) { await request(`/api/projects/${project.id}/environments`,{name});setEnvironment(name);closeSelection(); }
      if (modal === 'service' && project && serviceKind==='volume') { const volume=await request<Volume>(`/api/projects/${project.id}/volumes`,{name,environment}); setServiceID('');setDeploymentID('');setLocationResource(`volume:${volume.id}`); }
      else if (modal === 'service' && project && serviceKind==='bucket') { const bucket=await request<Bucket>(`/api/projects/${project.id}/buckets`,{name,environment,endpoint:bucketEndpoint,region:bucketRegion,bucketName:remoteBucket,accessKeyId:bucketAccessKey,secretAccessKey:bucketSecretKey,forcePathStyle:bucketPathStyle}); setServiceID('');setDeploymentID('');setLocationResource(`bucket:${bucket.id}`); }
      else if (modal === 'service' && project) { const dataTemplate=['postgres','redis','mysql','mongo'].includes(serviceKind); const s = await request<Service>(dataTemplate?`/api/projects/${project.id}/databases`:`/api/projects/${project.id}/resources`, dataTemplate?{name,environment,template:serviceKind}:{name,environment,sourceType,workloadMode}); setServiceID(s.id); setDeploymentID(''); setLocationResource(`service:${s.id}`); setTab(s.resourceKind==='database'?'settings':sourceType==='github'?'source':'deployments'); openImageDeploy=!dataTemplate&&sourceType==='image'; }
      if (modal === 'deploy' && service) { const d = await request<Deployment>(`/api/services/${service.id}/deployments`, { image: image.trim(), port: Number(port), healthPath: health }); setDeploymentID(d.id); setTab('deployments'); }
      close(); await refresh();
      if (openImageDeploy) { requestKey.current=crypto.randomUUID(); setImage(''); setPort('80'); setHealth('/'); setModal('deploy'); }
    } catch (err) { setFormError(err instanceof Error ? err.message : 'Something went wrong'); }
    finally { setBusy(false); }
  };
  const signOut = async () => { await request('/auth/logout',{});onLogout(); };

  return <div className="app-shell">
    <aside className="sidebar"><Brand /><div className="workspace-label"><span className="avatar">P</span><div>Personal workspace<small>Self-hosted</small></div><span className="workspace-check"><Check size={13} /></span></div><div className="sidebar-section"><span>WORKSPACE</span></div><div className="nav-current"><Layers3 size={17} /> Projects <span className="count">{data.projects.length}</span></div><div className="sidebar-section projects-heading"><span>YOUR PROJECTS</span><button className="icon-button" aria-label="Create project" onClick={() => open('project')}><Plus size={15} /></button></div><div className="project-nav">{data.projects.map(p => <button key={p.id} className={p.id === project?.id ? 'selected' : ''} onClick={() => { setProjectID(p.id); setEnvironment('production'); closeSelection(); }}><Folder size={16} /><span>{p.name}</span>{p.id === project?.id && <ChevronRight size={14} />}</button>)}{!data.projects.length && <p className="sidebar-empty">Your next idea starts here.</p>}</div><div className="sidebar-bottom"><div className="node-card"><span className={`node-indicator ${error ? 'offline' : ''}`} /><div>Your workspace<small>{error ? 'Connection interrupted' : ready ? 'Control plane connected' : 'Connecting…'}</small></div><Server size={16} /></div><button className="signout" onClick={signOut}><LogOut size={15} /> Sign out<span>v0.1 alpha</span></button></div></aside>
    <div className="workspace">
      <header className="topbar"><div className="breadcrumbs"><span>Personal</span><ChevronRight size={13} /><strong>{project?.name || 'Projects'}</strong></div><div className="topbar-right"><button className="text-button" onClick={()=>setGithubOpen(true)}>GitHub</button><span className="local-tag"><span /> Self-hosted alpha</span><span className="avatar small">P</span><button className="icon-button mobile-signout" aria-label="Sign out" onClick={signOut}><LogOut size={14} /></button></div></header>
      {error && <div className="connection-error" role="alert">{error} · Retrying automatically. Displayed data may be out of date.</div>}
      {!ready ? <div className="loading-page"><LoaderCircle className="spin" /> Connecting to your workspace…</div> : !project ? <div className="welcome"><div className="welcome-illustration"><Layers3 size={45} /></div><div className="eyebrow">START SOMETHING GOOD</div><h1>Your ideas, ready for takeoff.</h1><p>Create a project to give your services a home.<br />Deploy on your own server, from one place.</p><button className="primary" onClick={() => open('project')}><Plus size={16} /> Create your first project</button><div className="welcome-steps"><span><Folder size={17} /> Create a project</span><ChevronRight size={15} /><span><Box size={17} /> Add a service</span><ChevronRight size={15} /><span><Rocket size={17} /> Deploy an image</span></div></div> : <>
      <div className="project-header"><div><div className="eyebrow">PROJECT CANVAS</div><h1>{project.name}</h1><p>Build and operate the whole stack from one canvas.</p></div><button className="primary" onClick={() => setCreateOpen(true)}><Plus size={16} /> New resource</button></div>
      <div className="project-toolbar"><label className="environment environment-select"><GitBranch size={14} /><select aria-label="Environment" value={environment} onChange={e=>{setEnvironment(e.target.value);closeSelection()}}>{data.environments.filter(e=>e.projectId===project.id).map(e=><option key={e.name} value={e.name}>{e.name}</option>)}</select></label><button className="icon-button" aria-label="Create environment" onClick={()=>open('environment')}><Plus size={14}/></button><span className="toolbar-divider" /><div className="toolbar-stat"><span className="green-dot" />{running} running</div>{pending > 0 && <div className="toolbar-stat"><LoaderCircle size={12} className="spin" />{pending} deploying</div>}<div className="toolbar-spacer" /><span className="canvas-shortcut">⌘K create</span><NodeStatus/></div>
      <div className="project-body">
        <ProjectCanvas projectID={project.id} environment={environment} request={request} selectedKey={selectedKey} createOpen={createOpen} setCreateOpen={setCreateOpen} onCreate={beginCreate} onSelect={selectResource} onCloseSelection={closeSelection}/>
        {service && <section className="detail-pane" aria-label={`${service.name} details`}><div className="detail-header"><span className={`service-icon ${isDataService(service)?'database':''}`}>{isDataService(service)?<Database size={21}/>:<Box size={21} />}</span><div><h2>{service.name}</h2><span className="muted">{environment} / {isDataService(service)?`${dataName(service)} ${service.templateVersion||''} data service`:service.workloadMode==='worker'?'Background worker':service.workloadMode==='cron'?'Cron job':'Web / API service'}</span></div><button className="icon-button detail-close" aria-label="Close service details" onClick={closeSelection}><X size={17}/></button>{(service.workloadMode!=='cron'||service.cronSchedule)&&<button className="primary compact" onClick={() => open('deploy')}><Rocket size={14} /> Deploy</button>}</div><div className="service-address">{isDataService(service)?<span>Private {dataName(service)} · db-{service.id}:{dataPort(service)}</span>:service.workloadMode==='worker'?<span><span className={active&&service.desiredState!=='stopped'?'green-dot':''}/>{active&&service.desiredState!=='stopped'?'Route-free worker process is active':service.desiredState==='stopped'?'Worker is stopped. Start it from Settings.':'Deploy an image or GitHub build to start this worker'}</span>:service.workloadMode==='cron'?<span>{service.cronSchedule?`${service.cronSchedule} UTC${service.cronNextRun?` · next ${new Date(service.cronNextRun).toLocaleString()}`:''}`:'Set a UTC schedule in Settings before deploying'}</span>:active && service.desiredState!=='stopped' ? <a href={service.url} target="_blank" rel="noreferrer"><span className="green-dot" />{service.url.replace(/^https?:\/\//,'')}<ExternalLink size={13} /></a> : <span><Circle size={11} /> {service.desiredState==='stopped'?'Service is stopped. Start it from Settings.':'Your URL will be ready after the first deployment'}</span>}</div><div className="detail-tabs" role="tablist" aria-label="Service views"><button role="tab" aria-selected={tab === 'deployments'} onClick={() => setTab('deployments')} className={tab === 'deployments' ? 'active' : ''}><Rocket size={14} /> Deployments <span>{deployments.length}</span></button><button role="tab" aria-selected={tab === 'logs'} onClick={() => setTab('logs')} className={tab === 'logs' ? 'active' : ''}><Terminal size={14} /> Runtime logs</button><button role="tab" aria-selected={tab==='source'} className={tab==='source'?'active':''} disabled={isDataService(service)} onClick={()=>setTab('source')}>Source</button><button role="tab" aria-selected={tab==='variables'} className={tab==='variables'?'active':''} disabled={isDataService(service)} onClick={()=>setTab('variables')}>Variables</button><button role="tab" aria-selected={tab==='settings'} className={tab==='settings'?'active':''} onClick={()=>setTab('settings')}>Settings</button></div>
          {tab==='source'?<SourcePanel key={service.id} serviceID={service.id} request={request}/>:tab==='variables'?<VariablesPanel key={service.id} serviceID={service.id} services={services} request={request}/>:tab==='settings'?<><SettingsPanel service={service} actions={data.actions.filter(a=>a.serviceId===service.id)} cronRuns={data.cronRuns.filter(run=>run.serviceId===service.id)} request={request} refresh={refresh}/><OperationsPanel key={service.id} service={service} services={data.services} request={request} refresh={refresh} pending={data.actions.some(a=>a.serviceId===service.id&&(a.status==='queued'||a.status==='running'))}/></>:!selected ? <div className="first-deploy"><div className="empty-icon"><Rocket size={28} /></div><h3>{service.workloadMode==='cron'&&!service.cronSchedule?'Schedule this job first.':'Ready when you are.'}</h3><p>{service.workloadMode==='cron'&&!service.cronSchedule?'Save a five-field UTC schedule in Settings before choosing the job image.':<>Choose an image and we’ll take care<br />of preparing this release.</>}</p>{service.workloadMode==='cron'&&!service.cronSchedule?<button className="primary" onClick={()=>setTab('settings')}>Open Settings <ArrowRight size={15}/></button>:<button className="primary" onClick={() => open('deploy')}>Deploy an image <ArrowRight size={15} /></button>}<span className="first-deploy-note"><ShieldCheck size={13} /> {service.workloadMode==='cron'?'Runs are isolated and never receive a public route':'Readiness checked before going live'}</span></div> : <div className="deployment-content">{!terminal.includes(selected.status)&&<button className="secondary" onClick={async()=>{try{await request(`/api/deployments/${selected.id}/cancel`,{});await refresh()}catch(e){setError((e as Error).message)}}}>Cancel deployment</button>}<div className="history-selector"><label htmlFor="release">Deployment</label><select id="release" value={selected.id} onChange={e => setDeploymentID(e.target.value)}>{deployments.map(d => <option key={d.id} value={d.id}>{d.id.slice(0, 8)} · {pretty[d.status]} · {new Date(d.createdAt).toLocaleString()}</option>)}</select><ChevronDown size={13} /></div>
          {tab === 'deployments' ? <><div className={`release-card ${selected.status}`}><div className="release-top"><Status value={selected.status} /><span className="release-id">{selected.id.slice(0, 8)}</span></div><h3>{imageName(selected.image)}</h3><p className="digest" title={selected.image}>{selected.image.split('@')[1]}</p><div className="release-meta"><span><Clock3 size={12} />{new Date(selected.createdAt).toLocaleString()}</span><span>{service.workloadMode==='cron'?'Scheduled job':`Port ${selected.port}`}</span></div>{selected.settings?.startCommand&&<p className="deployment-runtime"><strong>Start</strong> {selected.settings.startCommand}</p>}{selected.settings?.preDeployCommand&&<p className="deployment-runtime"><strong>Pre-deploy</strong> {selected.settings.preDeployCommand} · {selected.settings.preDeployTimeoutSeconds}s</p>}{service.workloadMode!=='cron'&&<p className="deployment-runtime"><strong>Restart</strong> {selected.settings?.restartPolicy||'on-failure'}{(selected.settings?.restartPolicy||'on-failure')==='on-failure'?` · ${selected.settings?.restartMaxRetries||10} retries`:''}</p>}</div>{selected.error && <div className="failure-message" role="status"><strong>This deployment didn’t go live.</strong><p>{selected.error}</p>{active && <span><ShieldCheck size={13} /> Previous release is still selected for traffic.</span>}</div>}<div className="activity-heading"><h3><Activity size={14} /> Deployment activity</h3><span>Auto-refreshes</span></div><ol className="timeline">{events.map(e => <li key={e.id}><span className={`timeline-point ${e.stage}`}>{e.stage === 'failed' ? <X size={11} /> : <Check size={10} />}</span><div><strong>{pretty[e.stage]}</strong><p>{e.message}</p></div><time>{timestamp(e.createdAt)}</time></li>)}</ol>{terminal.includes(selected.status) && <button className="secondary redeploy" onClick={() => open('deploy', selected)}><ArrowDownToLine size={14} />{selected.status === 'superseded' ? 'Deploy this version again' : 'Redeploy image'}</button>}</> : <div className="logs-view"><div className="logs-toolbar"><span><Terminal size={13} /> stdout / stderr</span><span>Last 80 lines</span></div><pre>{selected.logs || 'No runtime logs captured for this deployment yet.'}</pre><p className="logs-note">Active release logs refresh while the deployment worker is idle. Avoid logging secrets.</p></div>}
          </div>}
        </section>}
      </div></>}
      <footer className="workspace-footer"><span><Radio size={12} /> {error ? 'Reconnecting' : 'Self-hosted control plane'}</span><span>Built to give you control.</span></footer>
    </div>
    {githubOpen&&<GitHubSettings request={request} close={()=>setGithubOpen(false)}/>}
    {modal && <Dialog title={modal === 'environment' ? 'Create environment' : modal === 'project' ? 'Create a project' : modal === 'service' ? createCopy[createIntent].title : 'Deploy an image'} description={modal === 'environment' ? 'A separate home for staging or another deployment configuration.' : modal === 'project' ? 'A home for the services that belong together.' : modal === 'service' ? createCopy[createIntent].description : service?.settings.mountPath ? 'The current container stops before the persistent replacement starts.' : 'We’ll check the new release before switching traffic.'} close={close}><form onSubmit={submit}>{modal !== 'deploy' ? <><label htmlFor="resource-name">{modal === 'project' ? 'Project' : modal==='environment'?'Environment':'Resource'} name</label><input id="resource-name" value={name} onChange={e => setName(e.target.value)} maxLength={60} required pattern="[a-zA-Z0-9][a-zA-Z0-9 _.\-]*" placeholder={modal === 'project' ? 'My next great idea' : createIntent==='postgres'?'postgres':createIntent==='redis'?'redis':createIntent==='mysql'?'mysql':createIntent==='mongo'?'mongo':createIntent==='volume'||createIntent==='bucket'?'uploads':'api-server'} />{modal === 'service' && <><div className="selected-resource-type"><span>{serviceKind==='volume'?<HardDrive size={17}/>:serviceKind==='bucket'?<Cloud size={17}/>:['postgres','redis','mysql','mongo'].includes(serviceKind)?<Database size={17}/>:<Box size={17}/>}</span><div><small>Resource type</small><strong>{serviceKind==='postgres'?'PostgreSQL database':serviceKind==='redis'?'Redis data service':serviceKind==='mysql'?'MySQL database':serviceKind==='mongo'?'MongoDB database':serviceKind==='volume'?'Persistent volume':serviceKind==='bucket'?'S3-compatible bucket':workloadMode==='worker'?'Background worker':workloadMode==='cron'?'Cron job':'Web / API service'}</strong></div><em>{project?.name} / {environment}</em></div>{serviceKind==='bucket'&&<div className="bucket-create-fields"><label htmlFor="bucket-endpoint">S3 endpoint</label><input id="bucket-endpoint" type="url" value={bucketEndpoint} onChange={e=>setBucketEndpoint(e.target.value)} required placeholder="https://s3.example.com"/><div className="form-row"><div><label htmlFor="bucket-region">Region</label><input id="bucket-region" value={bucketRegion} onChange={e=>setBucketRegion(e.target.value)} maxLength={100}/></div><div><label htmlFor="bucket-name">Remote bucket name</label><input id="bucket-name" value={remoteBucket} onChange={e=>setRemoteBucket(e.target.value)} required minLength={3} maxLength={63}/></div></div><label htmlFor="bucket-access">Access key ID</label><input id="bucket-access" value={bucketAccessKey} onChange={e=>setBucketAccessKey(e.target.value)} required minLength={3} maxLength={256} autoComplete="off"/><label htmlFor="bucket-secret">Secret access key</label><input id="bucket-secret" type="password" value={bucketSecretKey} onChange={e=>setBucketSecretKey(e.target.value)} required minLength={8} maxLength={1024} autoComplete="new-password"/><label className="checkbox-row"><input type="checkbox" checked={bucketPathStyle} onChange={e=>setBucketPathStyle(e.target.checked)}/> Force path-style URLs (MinIO and most self-hosted providers)</label></div>}{serviceKind==='http'&&<div className="form-row compute-kind-row"><div><label htmlFor="workload-mode">Workload</label><select id="workload-mode" value={workloadMode} onChange={e=>setWorkloadMode(e.target.value as typeof workloadMode)}><option value="web">Web / API</option><option value="worker">Background worker</option><option value="cron">Cron job</option></select></div><div><label htmlFor="source-type">Source</label><select id="source-type" value={sourceType} onChange={e=>setSourceType(e.target.value as typeof sourceType)}><option value="github">GitHub repository</option><option value="image">Docker image</option><option value="empty">Configure later</option></select></div></div>}</>}</> : <><label htmlFor="image">Container image</label><input id="image" readOnly={isDataService(service)} value={image} onChange={e => setImage(e.target.value)} required maxLength={512} placeholder="repository@sha256:…" autoComplete="off" /><p className="field-help">Public image with an immutable SHA-256 digest. Tags alone aren’t supported yet.</p><div className="form-row"><div><label htmlFor="port">Container port</label><input id="port" readOnly={isDataService(service)} type="number" min={1} max={65535} value={port} onChange={e => setPort(e.target.value)} required /></div><div><label htmlFor="health">Readiness path</label><input id="health" value={health} onChange={e => setHealth(e.target.value)} maxLength={200} required placeholder="/health" /></div></div><div className="form-note"><ShieldCheck size={16} /><span>Uses current service variables and resource settings. {service?.settings.mountPath ? 'Persistent services pause while replacing their container. Image rollback does not restore stored data.' : 'The current release stays online until the candidate is ready.'} Limits: {service?.settings.memoryMB ?? 256} MB RAM, {(service?.settings.cpuMillis ?? 1000) / 1000} CPU.</span></div></>}{formError && <p className="error-text" role="alert">{formError}</p>}<div className="modal-actions"><button type="button" className="secondary" onClick={close} disabled={busy}>Cancel</button><button className="primary" type="submit" disabled={busy}>{busy ? <LoaderCircle size={15} className="spin" /> : modal === 'deploy' ? <Rocket size={15} /> : <Plus size={15} />}{busy ? 'Saving…' : modal === 'project' ? 'Create project' : modal === 'environment'?'Create environment':modal === 'service' ? 'Create resource' : 'Deploy image'}</button></div></form></Dialog>}
  </div>;
}

createRoot(document.getElementById('root')!).render(<React.StrictMode><OwnerGate>{logout=><App onLogout={logout}/>}</OwnerGate></React.StrictMode>);
