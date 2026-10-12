import { test } from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";
const m=createRequire(import.meta.url)("../../public/meshmapper.js");
const cell={bounds:{south:43.46,north:43.4627,west:-80.38,east:-80.37616},snr:null,fill_color:"#1e7e34"};
const cache={source:"MeshMapper",schema_version:2,region:"YKF",available:true,cells:[cell],generated_at:1791832854};
test("cache isolation, documented bounds and nullable SNR remain intact",()=>{
  const result=m.normalize(cache,"YKF");assert.equal(result.cells[0],cell);assert.equal(result.cells[0].snr,null);
  assert.throws(()=>m.normalize(cache,"YYZ"));assert.equal(m.validBounds({...cell.bounds,north:43}),false);
  assert.equal(m.colour("url(https://wrong.example/)","grey"),"grey");
});
test("sparse culling uses source bounds and refuses a misleading visible subset",()=>{
  const rows=m.indexCells([cell]);assert.equal(m.visibleCells(rows,{south:43.461,north:43.47,west:-80.39,east:-80.37}).cells.length,1);
  assert.equal(m.visibleCells(rows,{south:42,north:42.1,west:-80.39,east:-80.37}).cells.length,0);
  const large=m.indexCells(Array.from({length:100000},()=>cell));
  const view=m.visibleCells(large,{south:43,north:44,west:-81,east:-79});assert.equal(view.overLimit,true);assert.equal(view.cells.length,0);
});
test("freshness, failures, missing and capped data are labelled distinctly",()=>{
  const now=1791832854*1000;
  assert.equal(m.timestampMs(1791832854000),now);assert.equal(m.timestampMs(null),null);
  assert.match(m.describe(cache,null,null,now+25*3600000),/Stale cache/);
  assert.doesNotMatch(m.describe(cache,{last_success:new Date(now+25*3600000).toISOString(),http_status:304},null,now+25*3600000),/Stale cache/);
  assert.match(m.describe({...cache,truncated:true},{error:"upstream_http_503"},{cells:[],overLimit:true},now),/Partial snapshot/);
  assert.match(m.describe(cache,{missing_regions:["YKF"]},null,now),/earlier snapshot/);
  assert.match(m.describe({...cache,available:false},null,null,now),/does not prove/);
  assert.match(m.describe(cache,null,null,now),/All reported radio configurations/);
});
