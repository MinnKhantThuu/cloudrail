import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import {
  Box,
  Cloud,
  Code2,
  Container,
  Database,
  Github,
  Grip,
  HardDrive,
  Maximize2,
  Network,
  Plus,
  Search,
  ServerCog,
  Timer,
  X,
  ZoomIn,
  ZoomOut,
} from 'lucide-react';

type Request = <T,>(path: string, body?: unknown, method?: string) => Promise<T>;

export type CreateIntent = 'github' | 'image' | 'empty' | 'worker' | 'cron' | 'postgres';

type Position = { x: number; y: number };
export type CanvasResource = {
  position: Position;
  key: string;
  id: string;
  kind: 'service' | 'database' | 'volume' | 'bucket';
  name: string;
  status: string;
  workloadMode?: string;
  sourceType?: string;
  template?: string;
  publicAddress?: string;
  privateAddress?: string;
};
type CanvasLink = { id: string; from: string; to: string; kind: string; label?: string };
type CanvasGraph = { projectId: string; environment: string; resources: CanvasResource[]; links: CanvasLink[] };

const nodeWidth = 250;
const nodeHeight = 136;
const emptyGraph: CanvasGraph = { projectId: '', environment: '', resources: [], links: [] };
const busyStatuses = ['queued', 'pulling', 'starting', 'checking', 'routing'];
const statusLabels: Record<string, string> = {
  queued: 'Queued', pulling: 'Pulling', starting: 'Starting', checking: 'Checking', routing: 'Routing',
  active: 'Active', failed: 'Failed', superseded: 'Replaced', stopped: 'Stopped', empty: 'Not deployed',
  available: 'Available', attached: 'Attached', crashed: 'Crashed',
};

function resourceIcon(resource: CanvasResource) {
  if (resource.kind === 'database') return <Database size={19} />;
  if (resource.kind === 'volume') return <HardDrive size={19} />;
  if (resource.kind === 'bucket') return <Cloud size={19} />;
  if (resource.workloadMode === 'cron') return <Timer size={19} />;
  if (resource.workloadMode === 'worker') return <ServerCog size={19} />;
  return <Box size={19} />;
}

function resourceSubtitle(resource: CanvasResource) {
  if (resource.kind === 'database') return `${resource.template || 'Database'} template`;
  if (resource.kind === 'volume') return 'Persistent volume';
  if (resource.kind === 'bucket') return 'S3-compatible bucket';
  if (resource.workloadMode === 'worker') return 'Background worker';
  if (resource.workloadMode === 'cron') return 'Cron job';
  return 'Web / API service';
}

function sourceLabel(resource: CanvasResource) {
  if (resource.kind === 'volume') return 'Attached storage';
  if (resource.kind === 'bucket') return resource.template || 'S3';
  if (resource.template) return resource.template;
  if (resource.sourceType === 'github') return 'GitHub repository';
  if (resource.sourceType === 'image') return 'Docker image';
  return 'Empty service';
}

function CreateOption({ icon, title, description, phase, onClick }: {
  icon: ReactNode;
  title: string;
  description: string;
  phase?: string;
  onClick?: () => void;
}) {
  return <button className="create-option" disabled={!onClick} onClick={onClick}>
    <span className="create-option-icon">{icon}</span>
    <span><strong>{title}</strong><small>{description}</small></span>
    {phase ? <em>{phase}</em> : <span className="create-ready">Ready</span>}
  </button>;
}

function CreatePalette({ open, close, create }: { open: boolean; close: () => void; create: (intent: CreateIntent) => void }) {
  useEffect(() => {
    if (!open) return;
    const key = (event: KeyboardEvent) => { if (event.key === 'Escape') close(); };
    document.addEventListener('keydown', key);
    return () => document.removeEventListener('keydown', key);
  }, [open, close]);
  if (!open) return null;
  return <div className="create-palette-shade" onPointerDown={event => { if (event.target === event.currentTarget) close(); }}>
    <section className="create-palette" role="dialog" aria-modal="true" aria-labelledby="create-title">
      <header><div><span className="eyebrow">PROJECT RESOURCE</span><h2 id="create-title">Add to your canvas</h2></div><button className="icon-button" aria-label="Close resource palette" onClick={close}><X size={18} /></button></header>
      <p className="create-palette-intro">Choose how this resource should run. Unavailable types show the phase that will enable them.</p>
      <h3>Compute</h3>
      <div className="create-option-grid">
        <CreateOption icon={<Github size={18} />} title="GitHub Repository" description="Create a service, then select repo and branch" onClick={() => create('github')} />
        <CreateOption icon={<Container size={18} />} title="Docker Image" description="Deploy an immutable container image" onClick={() => create('image')} />
        <CreateOption icon={<Plus size={18} />} title="Empty Service" description="Create now and configure source later" onClick={() => create('empty')} />
        <CreateOption icon={<ServerCog size={18} />} title="Background Worker" description="Long-running process without a public route" onClick={() => create('worker')} />
        <CreateOption icon={<Timer size={18} />} title="Cron Job" description="Run a command on a UTC schedule" onClick={() => create('cron')} />
      </div>
      <h3>Data</h3>
      <div className="create-option-grid">
        <CreateOption icon={<Database size={18} />} title="PostgreSQL" description="Private database with persistent storage" onClick={() => create('postgres')} />
        <CreateOption icon={<Database size={18} />} title="Redis" description="Cache, queue and key-value data" phase="UX-4" />
        <CreateOption icon={<Database size={18} />} title="MySQL" description="Persistent relational database" phase="UX-4" />
        <CreateOption icon={<Database size={18} />} title="MongoDB" description="Persistent document database" phase="UX-4" />
      </div>
      <h3>Storage</h3>
      <div className="create-option-grid">
        <CreateOption icon={<HardDrive size={18} />} title="Volume" description="Attach persistent storage to a service" phase="UX-4" />
        <CreateOption icon={<Cloud size={18} />} title="S3-compatible Bucket" description="Private object storage credentials" phase="UX-4" />
      </div>
    </section>
  </div>;
}

function ResourceDrawer({ resource, links, resources, close, select }: {
  resource: CanvasResource;
  links: CanvasLink[];
  resources: CanvasResource[];
  close: () => void;
  select: (resource: CanvasResource) => void;
}) {
  const related = links.filter(link => link.from === resource.key || link.to === resource.key).map(link => {
    const key = link.from === resource.key ? link.to : link.from;
    return { link, resource: resources.find(item => item.key === key) };
  });
  return <section className="detail-pane canvas-resource-drawer" aria-label={`${resource.name} details`}>
    <div className="detail-header"><span className={`service-icon ${resource.kind}`}>{resourceIcon(resource)}</span><div><h2>{resource.name}</h2><span className="muted">{resourceSubtitle(resource)}</span></div><button className="icon-button detail-close" aria-label="Close resource details" onClick={close}><X size={17} /></button></div>
    <div className="resource-summary">
      <div><span>Status</span><strong className={`resource-status ${resource.status}`}>{statusLabels[resource.status] || resource.status}</strong></div>
      <div><span>Resource key</span><code>{resource.key}</code></div>
    </div>
    <div className="resource-drawer-content"><h3>Connections</h3>{related.length ? related.map(({ link, resource: item }) => <button key={link.id} className="resource-link-row" disabled={!item} onClick={() => item && select(item)}><Network size={15} /><span><strong>{item?.name || 'Unavailable resource'}</strong><small>{link.kind} · {link.label}</small></span></button>) : <p className="muted">No recorded resource connection.</p>}
      <h3>Management</h3><p className="muted">Volume attach, backup and restore controls move into this drawer in UX-4. The current attachment remains fully active.</p>
    </div>
  </section>;
}

export function ProjectCanvas({ projectID, environment, request, selectedKey, createOpen, setCreateOpen, onCreate, onSelect, onCloseSelection }: {
  projectID: string;
  environment: string;
  request: Request;
  selectedKey: string;
  createOpen: boolean;
  setCreateOpen: (open: boolean) => void;
  onCreate: (intent: CreateIntent) => void;
  onSelect: (resource: CanvasResource, navigate?: boolean) => void;
  onCloseSelection: () => void;
}) {
  const [graph, setGraph] = useState<CanvasGraph>(emptyGraph);
  const [query, setQuery] = useState('');
  const [error, setError] = useState('');
  const [zoom, setZoom] = useState(1);
  const [pan, setPan] = useState({ x: 0, y: 0 });
  const viewport = useRef<HTMLDivElement>(null);
  const drag = useRef<{ key: string; startX: number; startY: number; origin: Position; position: Position; moved: boolean } | null>(null);
  const panning = useRef<{ startX: number; startY: number; origin: Position } | null>(null);
  const suppressClick = useRef(false);
  const path = `/api/projects/${projectID}/environments/${encodeURIComponent(environment)}/canvas`;

  const load = useCallback(async () => {
    try {
      const next = await request<CanvasGraph>(path);
      if (!drag.current) setGraph(next);
      setError('');
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Canvas unavailable');
    }
  }, [path, request]);

  useEffect(() => {
    let stopped = false;
    const poll = async () => { if (!stopped) await load(); };
    void poll();
    const timer = setInterval(poll, 2500);
    return () => { stopped = true; clearInterval(timer); };
  }, [load]);

  useEffect(() => {
    const shortcut = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
        event.preventDefault();
        setCreateOpen(true);
      }
    };
    document.addEventListener('keydown', shortcut);
    return () => document.removeEventListener('keydown', shortcut);
  }, [setCreateOpen]);

  useEffect(() => {
    const selected = graph.resources.find(resource => resource.key === selectedKey);
    if (selected && (selected.kind === 'service' || selected.kind === 'database')) onSelect(selected, false);
  }, [graph.projectId, selectedKey]);

  const visible = useMemo(() => graph.resources.filter(resource => resource.name.toLowerCase().includes(query.toLowerCase())), [graph.resources, query]);
  const visibleKeys = useMemo(() => new Set(visible.map(resource => resource.key)), [visible]);
  const resourceByKey = useMemo(() => new Map(graph.resources.map(resource => [resource.key, resource])), [graph.resources]);
  const selectedResource = resourceByKey.get(selectedKey);
  const bounds = useMemo(() => {
    const points = graph.resources.map(resource => resource.position);
    return { width: Math.max(1500, ...points.map(point => point.x + nodeWidth + 180)), height: Math.max(900, ...points.map(point => point.y + nodeHeight + 180)) };
  }, [graph.resources]);

  const fit = () => {
    if (!viewport.current || !graph.resources.length) { setZoom(1); setPan({ x: 0, y: 0 }); return; }
    const xs = graph.resources.map(resource => resource.position.x);
    const ys = graph.resources.map(resource => resource.position.y);
    const left = Math.min(...xs), top = Math.min(...ys), right = Math.max(...xs) + nodeWidth, bottom = Math.max(...ys) + nodeHeight;
    const rect = viewport.current.getBoundingClientRect();
    const nextZoom = Math.max(.55, Math.min(1.2, (rect.width - 120) / (right - left), (rect.height - 120) / (bottom - top)));
    setZoom(nextZoom);
    setPan({ x: (rect.width - (right - left) * nextZoom) / 2 - left * nextZoom, y: (rect.height - (bottom - top) * nextZoom) / 2 - top * nextZoom });
  };

  const select = (resource: CanvasResource) => onSelect(resource, true);

  return <section className="canvas-pane" aria-label="Project canvas">
    <div className="canvas-toolbar">
      <div className="canvas-heading"><Network size={15} /><strong>Canvas</strong><span>{graph.resources.length} resources</span></div>
      <label className="canvas-search"><Search size={14} /><input value={query} onChange={event => setQuery(event.target.value)} aria-label="Filter resources" placeholder="Find a resource" /></label>
      <div className="canvas-controls"><button className="icon-button" aria-label="Zoom out" onClick={() => setZoom(value => Math.max(.5, value - .1))}><ZoomOut size={16} /></button><span>{Math.round(zoom * 100)}%</span><button className="icon-button" aria-label="Zoom in" onClick={() => setZoom(value => Math.min(1.5, value + .1))}><ZoomIn size={16} /></button><button className="icon-button" aria-label="Fit canvas" onClick={fit}><Maximize2 size={15} /></button><button className="primary compact canvas-create" onClick={() => setCreateOpen(true)}><Plus size={14} /> Create</button></div>
    </div>
    {error && <div className="canvas-error" role="alert">{error} · Retrying automatically.</div>}
    <div className="canvas-viewport" ref={viewport} onContextMenu={event => { event.preventDefault(); setCreateOpen(true); }} onWheel={event => { if (event.ctrlKey || event.metaKey) return; event.preventDefault(); setZoom(value => Math.max(.5, Math.min(1.5, value - Math.sign(event.deltaY) * .08))); }} onPointerDown={event => { if ((event.target as Element).closest('button')) return; event.currentTarget.setPointerCapture(event.pointerId); panning.current = { startX: event.clientX, startY: event.clientY, origin: pan }; }} onPointerMove={event => { if (!panning.current) return; setPan({ x: panning.current.origin.x + event.clientX - panning.current.startX, y: panning.current.origin.y + event.clientY - panning.current.startY }); }} onPointerUp={event => { if (panning.current) event.currentTarget.releasePointerCapture(event.pointerId); panning.current = null; }}>
      {!graph.resources.length && !error ? <div className="canvas-empty"><span><Plus size={23} /></span><h2>Build your project on the canvas</h2><p>Add a repository, Docker image, empty service or database.</p><button className="primary" onClick={() => setCreateOpen(true)}><Plus size={15} /> Create resource</button></div> : <div className="canvas-stage" style={{ width: bounds.width, height: bounds.height, transform: `translate(${pan.x}px, ${pan.y}px) scale(${zoom})` }}>
        <svg className="canvas-links" width={bounds.width} height={bounds.height} aria-hidden="true"><defs><marker id="canvas-arrow" markerWidth="8" markerHeight="8" refX="7" refY="4" orient="auto"><path d="M0,0 L8,4 L0,8 z" /></marker></defs>{graph.links.filter(link => visibleKeys.has(link.from) && visibleKeys.has(link.to)).map(link => {
          const from = resourceByKey.get(link.from), to = resourceByKey.get(link.to);
          if (!from || !to) return null;
          const dx = to.position.x - from.position.x, dy = to.position.y - from.position.y;
          const vertical = Math.abs(dy) > Math.abs(dx);
          let x1: number, y1: number, x2: number, y2: number, pathData: string;
          if (vertical) {
            const direction = dy >= 0 ? 1 : -1;
            x1 = from.position.x + nodeWidth / 2; y1 = from.position.y + (direction > 0 ? nodeHeight : 0);
            x2 = to.position.x + nodeWidth / 2; y2 = to.position.y + (direction > 0 ? 0 : nodeHeight);
            const curve = Math.max(55, Math.abs(y2 - y1) / 2);
            pathData = `M ${x1} ${y1} C ${x1} ${y1 + direction * curve}, ${x2} ${y2 - direction * curve}, ${x2} ${y2}`;
          } else {
            const direction = dx >= 0 ? 1 : -1;
            x1 = from.position.x + (direction > 0 ? nodeWidth : 0); y1 = from.position.y + nodeHeight / 2;
            x2 = to.position.x + (direction > 0 ? 0 : nodeWidth); y2 = to.position.y + nodeHeight / 2;
            const curve = Math.max(70, Math.abs(x2 - x1) / 2);
            pathData = `M ${x1} ${y1} C ${x1 + direction * curve} ${y1}, ${x2 - direction * curve} ${y2}, ${x2} ${y2}`;
          }
          return <g key={link.id}><path d={pathData} markerEnd="url(#canvas-arrow)" /><title>{link.label || link.kind}</title></g>;
        })}</svg>
        {visible.map(resource => <button key={resource.key} className={`canvas-node ${resource.kind} ${selectedKey === resource.key ? 'selected' : ''}`} style={{ left: resource.position.x, top: resource.position.y }} aria-label={`${resource.name} resource`} onClick={() => { if (suppressClick.current) { suppressClick.current = false; return; } select(resource); }} onPointerDown={event => { event.stopPropagation(); event.currentTarget.setPointerCapture(event.pointerId); drag.current = { key: resource.key, startX: event.clientX, startY: event.clientY, origin: resource.position, position: resource.position, moved: false }; }} onPointerMove={event => { const current = drag.current; if (!current || current.key !== resource.key) return; const dx = (event.clientX - current.startX) / zoom, dy = (event.clientY - current.startY) / zoom; if (Math.abs(dx) + Math.abs(dy) > 5) current.moved = true; const position = { x: Math.round(Math.max(20, current.origin.x + dx)), y: Math.round(Math.max(20, current.origin.y + dy)) }; current.position = position; setGraph(value => ({ ...value, resources: value.resources.map(item => item.key === resource.key ? { ...item, position } : item) })); }} onPointerUp={event => { const current = drag.current; if (!current || current.key !== resource.key) return; event.currentTarget.releasePointerCapture(event.pointerId); drag.current = null; suppressClick.current = current.moved; if (current.moved) void request(path + '/layout', { positions: [{ resourceKey: resource.key, ...current.position }] }, 'PUT').catch(cause => setError(cause instanceof Error ? cause.message : 'Could not save layout')); }} onPointerCancel={() => { drag.current = null; }}>
          <span className="canvas-node-top"><span className="canvas-node-icon">{resourceIcon(resource)}</span><span><strong>{resource.name}</strong><small>{resourceSubtitle(resource)}</small></span><Grip size={14} /></span>
          <span className="canvas-node-source"><Code2 size={13} /> {sourceLabel(resource)}</span>
          <span className="canvas-node-bottom"><span className={`canvas-status ${resource.status}`}>{busyStatuses.includes(resource.status) && <span className="canvas-pulse" />}{statusLabels[resource.status] || resource.status}</span><small>{resource.publicAddress?.replace(/^https?:\/\//, '') || resource.privateAddress || resource.key.split(':')[0]}</small></span>
        </button>)}
        {!visible.length && <p className="canvas-no-results">No resources match “{query}”.</p>}
        <button className="canvas-add-node" style={{ left: 90, top: bounds.height - 100 }} onClick={() => setCreateOpen(true)}><Plus size={16} /> Add resource</button>
      </div>}
    </div>
    <div className="canvas-hint"><Grip size={13} /> Drag nodes to arrange · drag empty space to pan · right-click to create</div>
    {selectedResource && selectedResource.kind !== 'service' && selectedResource.kind !== 'database' && <ResourceDrawer resource={selectedResource} links={graph.links} resources={graph.resources} close={onCloseSelection} select={select} />}
    <CreatePalette open={createOpen} close={() => setCreateOpen(false)} create={onCreate} />
  </section>;
}
