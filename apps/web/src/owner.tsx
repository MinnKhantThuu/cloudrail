import { useCallback, useEffect, useState, type ReactNode } from 'react';
import { ArrowRight, Eye, EyeOff, Layers3 } from 'lucide-react';

type OwnerStatus = { configured: boolean; email: string };

export function OwnerGate({children}:{children:(logout:()=>void)=>ReactNode}) {
 const [status,setStatus]=useState<OwnerStatus|null>(null);
 const [email,setEmail]=useState('');
 const [password,setPassword]=useState('');
 const [confirmation,setConfirmation]=useState('');
 const [showPassword,setShowPassword]=useState(false);
 const [error,setError]=useState('');
 const [busy,setBusy]=useState(false);
 const load=useCallback(async()=>{
  try {
   const response=await fetch('/auth/status',{cache:'no-store'});
   if(!response.ok)throw new Error('Workspace unavailable');
   setStatus(await response.json());
  } catch(e) {setError(e instanceof Error?e.message:'Connection failed')}
 },[]);
 useEffect(()=>{sessionStorage.removeItem('cloudrail-token');void load()},[load]);
 const logout=useCallback(()=>{setStatus(s=>s?{...s,email:''}:s);setPassword('');setConfirmation('');setShowPassword(false);void load()},[load]);
 if(status?.email)return <>{children(logout)}</>;
 const setup=status!==null&&!status.configured;
 return <div className="login-page">
  <div className="login-header"><div className="brand"><span className="brand-icon"><Layers3 size={21}/></span>cloudrail</div><span className="preview-tag">YOUR OWN WORKSPACE</span></div>
  <main className="login-card">
   <div className="eyebrow">YOUR SERVER. YOUR SPACE.</div>
   <h1>{!status?'Opening your workspace…':setup?'Create your account':'Welcome back.'}</h1>
   <p className="login-intro">{setup?'Set up your owner account to start deploying.':'Sign in to build, deploy and manage your services.'}</p>
   <form onSubmit={async e=>{
    e.preventDefault();setError('');
    if(setup&&password!==confirmation){setError('Passwords do not match');return}
    setBusy(true);
    try {
     const response=await fetch(setup?'/auth/setup':'/auth/login',{method:'POST',headers:{'Content-Type':'application/json','X-Cloudrail-Request':'1'},body:JSON.stringify(setup?{email,password,passwordConfirmation:confirmation}:{email,password})});
     const data=await response.json();
     if(!response.ok){if(response.status===409)await load();throw new Error(data.error)}
     setPassword('');setConfirmation('');setShowPassword(false);await load();
    } catch(e){setError(e instanceof Error?e.message:'Sign-in failed')}
    finally{setBusy(false)}
   }}>
    <label htmlFor="owner-email">Email</label>
    <input id="owner-email" type="email" autoComplete="username" value={email} onChange={e=>setEmail(e.target.value)} required disabled={busy}/>
    <div className="password-label spaced-label"><label htmlFor="owner-password">Password</label><button type="button" className="text-button" onClick={()=>setShowPassword(s=>!s)} aria-label={showPassword?'Hide password':'Show password'}>{showPassword?<EyeOff size={16}/>:<Eye size={16}/>} {showPassword?'Hide':'Show'}</button></div>
    <input id="owner-password" type={showPassword?'text':'password'} minLength={setup?12:undefined} maxLength={72} autoComplete={setup?'new-password':'current-password'} value={password} onChange={e=>setPassword(e.target.value)} required disabled={busy}/>
    {setup&&<><p className="field-help">Use at least 12 characters.</p><label className="spaced-label" htmlFor="owner-confirmation">Confirm password</label><input id="owner-confirmation" type={showPassword?'text':'password'} maxLength={72} autoComplete="new-password" value={confirmation} onChange={e=>setConfirmation(e.target.value)} required disabled={busy}/></>}
    {error&&<p className="error-text" role="alert">{error}</p>}
    <button type="submit" className="primary full" disabled={busy||!status}>{busy?'Please wait…':setup?'Create account':'Sign in'}<ArrowRight size={16}/></button>
   </form>
   <p className="login-help">{setup?'Your account will own this Cloudrail installation.':'Use the account you created when setting up Cloudrail.'}</p>
   {!status&&error&&<button className="secondary full" onClick={()=>void load()}>Retry connection</button>}
  </main>
  <footer className="login-footer">Open source. On your terms.<span>Cloudrail</span></footer>
 </div>
}
