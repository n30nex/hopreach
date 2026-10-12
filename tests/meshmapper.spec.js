const { test,expect }=require("@playwright/test");
const { readFileSync }=require("node:fs");
const { join }=require("node:path");
const root=join(__dirname,"..");
const shell=readFileSync(join(root,"public/index.html"),"utf8");
const entries=[{iata:"YKF",name:"Waterloo",centerLat:43.46,centerLon:-80.38,zoom:9,radiusKm:200},{iata:"YYZ",name:"Toronto",centerLat:43.68,centerLon:-79.63,zoom:9,radiusKm:200}];
const config={siteName:"Canadaverse Coverage",meshMapperEnabled:true,regions:{defaultIata:"YKF",entries}};

async function fixture(page,missing=false) {
  const requests=[];
  const controls=shell.slice(shell.indexOf('<section id="regional-controls"'),shell.indexOf('<div id="error-banner"'));
  const replay=shell.slice(shell.indexOf('<div class="sim-promoted" id="sim-promoted-replay"'),shell.indexOf('<!-- Advanced-only:'));
  await page.route("http://measured.test/**",async route=>{
    const path=new URL(route.request().url()).pathname;requests.push(path);
    if(path.endsWith(".js")) return route.fulfill({contentType:"application/javascript",body:readFileSync(join(root,"public",path.slice(1)),"utf8")});
    if(path.endsWith("meshmapper-status.json")) return route.fulfill({json:{source:"MeshMapper",http_status:200,interval_hours:12}});
    if(path.endsWith("meshmapper.json")) {
      if(missing) return route.fulfill({status:404,body:"not ready"});
      const r=entries.find(r=>path.includes(`/${r.iata}/`));
      return route.fulfill({json:{source:"MeshMapper",region:r.iata,schema_version:2,available:true,generated_at:Math.floor(Date.now()/1000),cells:[{bounds:{south:r.centerLat,west:r.centerLon,north:r.centerLat+.0027,east:r.centerLon+.00384},fill_color:"#1e7e34",border_color:"#14522d",snr:null}]}});
    }
    return route.fulfill({contentType:"text/html",body:`<!doctype html><html><head><meta charset="utf-8"><style>.hidden,[hidden]{display:none!important}#map{height:400px;width:600px;position:relative}canvas{position:absolute}</style></head><body>
      <a id="canadaverse-brand" hidden>Canadaverse</a>${controls}${replay}<footer id="regional-footer" hidden>HopReach</footer><div id="map"></div><p id="attribution"></p>
      <script>window.HOPREACH_CONFIG=${JSON.stringify(config)}</script><script src="/regions.js"></script><script src="/meshmapper.js"></script>
      <script>
        const r=HOPREACH_CONFIG.regions.entries.find(r=>r.iata===HOPREACH_CONFIG.iata),pane=document.getElementById('map');
        const map={getSize:()=>({x:600,y:400}),getPanes:()=>({overlayPane:pane}),getPane:()=>pane,createPane:()=>pane,on(){},off(){},containerPointToLayerPoint:()=>({x:0,y:0}),
          getBounds:()=>({getSouth:()=>r.centerLat-.1,getNorth:()=>r.centerLat+.1,getWest:()=>r.centerLon-.1,getEast:()=>r.centerLon+.1}),
          latLngToContainerPoint:p=>({x:300+(p[1]-r.centerLon)*3000,y:200-(p[0]-r.centerLat)*2000}),removeLayer(layer){if(layer.map)layer.onRemove(this)},
          attributionControl:{addAttribution(s){document.getElementById('attribution').innerHTML=s},removeAttribution(){document.getElementById('attribution').textContent=''}}};
        window.L={Layer:{extend(methods){function Layer(){}Object.assign(Layer.prototype,methods,{addTo(map){if(!this.map)this.onAdd(map);return this}});return Layer}},DomUtil:{create(tag,cls){const el=document.createElement(tag);el.className=cls;return el},setPosition(){}}};
        HopReachMeshMapper.init({map,config:HOPREACH_CONFIG});
      </script></body></html>`});
  });
  return requests;
}

test("measured canvas is independent, attributed and loaded only once per region",async({page})=>{
  const requests=await fixture(page);await page.goto("http://measured.test/?iata=YKF");
  const toggle=page.getByLabel("Measured coverage · MeshMapper");
  await expect(page.locator("canvas")).toHaveCount(0);await toggle.check();
  await expect(page.locator("canvas")).toHaveCount(1);
  await expect(page.locator("#meshmapper-status")).toContainText("1 cells in this view");
  await expect(page.locator("#meshmapper-status")).toContainText("All reported radio configurations");
  await expect(page.locator("#attribution a")).toHaveAttribute("href","https://meshmapper.net/");
  await toggle.uncheck();await expect(page.locator("canvas")).toHaveCount(0);await toggle.check();await expect(page.locator("canvas")).toHaveCount(1);
  expect(requests.filter(p=>p.endsWith("/YKF/meshmapper.json"))).toHaveLength(1);
  await page.getByLabel("Canada / IATA region").selectOption("YYZ");
  await expect(page).toHaveURL(/iata=YYZ/);await expect(page.locator("canvas")).toHaveCount(1);
  expect(requests.filter(p=>p.endsWith("/YYZ/meshmapper.json"))).toHaveLength(1);
  expect(requests.every(p=>!p.includes("coverage.php")&&!p.includes("key"))).toBeTruthy();
});

test("missing measurement cache keeps the separate prediction contract visible",async({page})=>{
  await fixture(page,true);await page.goto("http://measured.test/?iata=YKF&measured=1");
  await expect(page.locator("#meshmapper-status")).toContainText("predicted terrain layer is still independent");
  await expect(page.locator("canvas")).toHaveCount(0);
});
