import {test,expect} from '@playwright/test';
import {readFileSync,existsSync} from 'node:fs';import {fileURLToPath} from 'node:url';
const fixture=new URL('../../../.data/phase5-result.json',import.meta.url);
const result=existsSync(fixture)?JSON.parse(readFileSync(fixture,'utf8')):null;
test.skip(!result,'Run scripts/phase5_acceptance.py to create this fixture.');
test('create a PostgreSQL service and manage a real backup in the dashboard',async({page})=>{
  const owner=JSON.parse(readFileSync(new URL('../../../.data/test-owner.json',import.meta.url),'utf8'));
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await page.goto('/');await page.getByLabel('Email',{exact:true}).fill(owner.email);await page.getByLabel('Password',{exact:true}).fill(owner.password);await page.getByRole('button',{name:'Sign in',exact:true}).click();
 await page.getByRole('button',{name:result.project.name,exact:true}).click();await page.locator('.project-header').getByRole('button',{name:'New service',exact:true}).click();const name='browser-db-'+Date.now().toString().slice(-5);const dialog=page.getByRole('dialog');await dialog.getByLabel('Service name').fill(name);await dialog.getByLabel('Service type').selectOption('postgres');await dialog.getByRole('button',{name:'Create service',exact:true}).click();
 await expect(page.getByRole('region',{name:name+' details'})).toBeVisible();await page.getByRole('tab',{name:'Settings',exact:true}).click();await expect(page.getByRole('button',{name:'Start',exact:true})).toBeEnabled({timeout:90000});await expect(page.getByLabel('Persistent volume path')).toHaveValue('/var/lib/postgresql/data');await expect(page.getByLabel('Persistent volume path')).toBeDisabled();
 await page.getByRole('button',{name:'Create backup',exact:true}).click();await expect(page.locator('.backup-list a')).toHaveCount(1,{timeout:30000});await expect(page.locator('.usage-grid')).toContainText('running');await page.getByLabel('Application',{exact:true}).selectOption(result.app.id);await page.getByRole('button',{name:'Connect application',exact:true}).click();await expect(page.getByRole('status').filter({hasText:'Connection saved.'})).toBeVisible();
 await page.screenshot({path:fileURLToPath(new URL('../../../.data/screenshots/database-operations.png',import.meta.url)),fullPage:true});await page.setViewportSize({width:390,height:844});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBeTruthy();expect(errors).toEqual([]);
});
