import {test,expect} from '@playwright/test';
import {readFileSync,existsSync} from 'node:fs';
import {fileURLToPath} from 'node:url';
const fixture=new URL('../../../.data/phase4-result.json',import.meta.url);
const result=existsSync(fixture)?JSON.parse(readFileSync(fixture,'utf8')):null;
test.skip(!result,'Run scripts/phase4_acceptance.py to create this fixture.');
test('source history and GitHub setup are usable on desktop and mobile',async({page})=>{
  const owner=JSON.parse(readFileSync(new URL('../../../.data/test-owner.json',import.meta.url),'utf8'));
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await page.goto('/');await page.getByLabel('Email',{exact:true}).fill(owner.email);await page.getByLabel('Password',{exact:true}).fill(owner.password);await page.getByRole('button',{name:'Sign in',exact:true}).click();
 await page.getByRole('button',{name:result.project.name,exact:true}).click();
 await page.getByRole('tab',{name:'Source',exact:true}).click();await expect(page.getByLabel('Repository',{exact:true})).toHaveValue('traefik/whoami');await expect(page.getByLabel('Build history')).toBeVisible();await expect(page.locator('.build-logs')).not.toBeEmpty();
 await page.screenshot({path:fileURLToPath(new URL('../../../.data/screenshots/source-builds.png',import.meta.url)),fullPage:true});
 await page.getByRole('button',{name:'GitHub',exact:true}).click();await expect(page.getByRole('dialog',{name:'Connect GitHub'})).toBeVisible();await expect(page.getByLabel('Webhook secret')).toHaveValue('');await page.getByLabel('Close GitHub settings').click();
 await page.setViewportSize({width:390,height:844});await page.screenshot({path:fileURLToPath(new URL('../../../.data/screenshots/source-mobile.png',import.meta.url)),fullPage:true});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBeTruthy();expect(errors).toEqual([]);
});
