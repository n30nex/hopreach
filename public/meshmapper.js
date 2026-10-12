// MeshMapper's measured grid is a separate cached layer. This module never
// contacts the protected upstream API and never receives its credential.
(function (root,factory) {
  if (typeof module === "object" && module.exports) module.exports=factory();
  else root.HopReachMeshMapper=factory();
})(typeof self !== "undefined" ? self : this,function () {
  "use strict";
  const ROW_DEGREES=0.05, MAX_CELLS=100000, VISIBLE_LIMIT=6000;

  function validBounds(b) {
    return b && [b.south,b.west,b.north,b.east].every(Number.isFinite) &&
      b.south>=-90 && b.north<=90 && b.south<b.north && b.west>=-180 && b.east<=180 && b.west<b.east &&
      b.north-b.south<=0.1 && b.east-b.west<=0.1;
  }
  function normalize(data,iata) {
    if (!data || data.source!=="MeshMapper" || data.region!==iata || data.schema_version!==2 ||
        !Array.isArray(data.cells) || data.cells.length>MAX_CELLS) throw new Error("Wrong or unsupported regional measurement cache");
    const cells=data.cells.filter(c=>c && validBounds(c.bounds));
    return Object.assign({},data,{cells,invalid_cells:(Number(data.invalid_cells)||0)+data.cells.length-cells.length});
  }
  function indexCells(cells) {
    const rows=new Map();
    for (const cell of cells) {
      const row=Math.floor((cell.bounds.south+cell.bounds.north)/2/ROW_DEGREES);
      if (!rows.has(row)) rows.set(row,[]);
      rows.get(row).push(cell);
    }
    return rows;
  }
  function visibleCells(rows,bounds,limit=VISIBLE_LIMIT) {
    const cells=[];
    const first=Math.max(-1800,Math.floor((bounds.south-0.1)/ROW_DEGREES));
    const last=Math.min(1800,Math.floor((bounds.north+0.1)/ROW_DEGREES));
    for (let row=first;row<=last;row++) for (const cell of rows.get(row)||[]) {
      const b=cell.bounds;
      if (b.north<bounds.south || b.south>bounds.north || b.east<bounds.west || b.west>bounds.east) continue;
      if (cells.length===limit) return {cells:[],overLimit:true};
      cells.push(cell);
    }
    return {cells,overLimit:false};
  }
  function timestampMs(value) {
    if (value==null || !Number.isFinite(Number(value)) || Number(value)<=0) return null;
    return Number(value)>2e10 ? Number(value) : Number(value)*1000;
  }
  function describe(data,status,view,now=Date.now()) {
    if (!data.available) return "No MeshMapper grid is available for this IATA region. This does not prove there is no coverage.";
    const generated=timestampMs(data.generated_at);
    const missing=status && (status.missing_regions||[]).includes(data.region);
    const checked=status && Date.parse(status.last_success);
    const validated=!missing && checked>0 ? checked : null;
    const freshness=validated || Date.parse(data.fetched_at) || generated;
    const notes=[`MeshMapper · ${data.cells.length.toLocaleString()} retained historical measurement cells.`];
    if (generated) notes.push(`Feed generated ${new Date(generated).toLocaleString()}.`);
    if (!freshness || now-freshness>24*3600000) notes.push("Stale cache: no successful validation in more than 24 hours.");
    if (status && status.error) {
      const running=status.error==="refresh_incomplete" && now-Date.parse(status.last_attempt)<10*60000;
      notes.push(running ? "Refresh in progress; cached measurements remain visible." : "Latest refresh failed; last good measurements are retained.");
    }
    if (missing) notes.push("Latest feed omitted this region; an earlier snapshot is retained.");
    if (data.truncated || data.invalid_cells>0) notes.push("Partial snapshot: cell cap or invalid source cells omitted data.");
    if (view && view.overLimit) notes.push("Zoom in to draw the grid: more than 6,000 cells are visible.");
    else if (view) notes.push(`${view.cells.length.toLocaleString()} cells in this view.`);
    if (!data.cells.length) notes.push("No retained cells does not prove there is no coverage.");
    notes.push("All reported radio configurations; separate from the terrain model’s radio assumptions.");
    return notes.join(" ");
  }
  function colour(value,fallback) {return /^#[0-9a-f]{6}$/i.test(value||"") ? value : fallback;}

  function canvasLayer(L,rows,onDraw) {
    return new (L.Layer.extend({
      onAdd(map) {
        this.map=map;this.canvas=L.DomUtil.create("canvas","leaflet-layer leaflet-zoom-hide meshmapper-canvas");
        this.canvas.style.pointerEvents="none";
        const pane=map.getPane("meshmapper")||map.createPane("meshmapper");
        pane.style.zIndex="430";pane.style.pointerEvents="none";pane.appendChild(this.canvas);
        map.on("moveend zoomend resize",this.schedule,this);this.schedule();
      },
      onRemove(map) {
        map.off("moveend zoomend resize",this.schedule,this);cancelAnimationFrame(this.frame);
        this.canvas.remove();this.map=null;
      },
      schedule() {
        cancelAnimationFrame(this.frame);this.frame=requestAnimationFrame(()=>this.draw());
      },
      draw() {
        if (!this.map) return;
        const size=this.map.getSize(),scale=Math.min(1,4096/size.x,4096/size.y);
        this.canvas.width=Math.ceil(size.x*scale);this.canvas.height=Math.ceil(size.y*scale);
        this.canvas.style.width=`${size.x}px`;this.canvas.style.height=`${size.y}px`;
        L.DomUtil.setPosition(this.canvas,this.map.containerPointToLayerPoint([0,0]));
        const ctx=this.canvas.getContext("2d");ctx.scale(scale,scale);
        const b=this.map.getBounds(),view=visibleCells(rows,{south:b.getSouth(),north:b.getNorth(),west:b.getWest(),east:b.getEast()});
        const fallback=getComputedStyle(document.documentElement).getPropertyValue("--status-silent").trim();
        for (const cell of view.cells) {
          const sw=this.map.latLngToContainerPoint([cell.bounds.south,cell.bounds.west]);
          const ne=this.map.latLngToContainerPoint([cell.bounds.north,cell.bounds.east]);
          ctx.fillStyle=colour(cell.fill_color,fallback);ctx.globalAlpha=0.55;ctx.fillRect(sw.x,ne.y,ne.x-sw.x,sw.y-ne.y);
          ctx.strokeStyle=colour(cell.border_color,fallback);ctx.globalAlpha=0.8;ctx.lineWidth=0.7;ctx.strokeRect(sw.x,ne.y,ne.x-sw.x,sw.y-ne.y);
        }
        onDraw(view);
      },
    }))();
  }

  function init({map,config}) {
    if (!config.meshMapperEnabled || !config.iata) return;
    const panel=document.getElementById("meshmapper-controls"),toggle=document.getElementById("meshmapper-toggle"),message=document.getElementById("meshmapper-status");
    panel.hidden=false;
    const attribution='<a href="https://meshmapper.net/">Measured coverage · MeshMapper</a>';
    let promise,layer;
    function update() {
      const url=new URL(location.href);url.searchParams.set("measured",toggle.checked?"1":"0");history.replaceState(null,"",url);
      if (!toggle.checked) {if(layer) map.removeLayer(layer);map.attributionControl.removeAttribution(attribution);return;}
      message.textContent="Loading cached MeshMapper measurements…";
      if (!promise) promise=Promise.all([
        fetch(`${config.dataBase}/meshmapper.json`).then(r=>{if(!r.ok) throw new Error("Measurement cache is not ready");return r.json();}),
        fetch("data/meshmapper-status.json").then(r=>r.ok?r.json():null).catch(()=>null),
      ]).then(([data,status])=>({data:normalize(data,config.iata),status}));
      promise.then(({data,status})=>{
        if (!layer) layer=canvasLayer(window.L,indexCells(data.cells),view=>{message.textContent=describe(data,status,view);});
        message.textContent=describe(data,status);
        if (toggle.checked) {layer.addTo(map);map.attributionControl.addAttribution(attribution);}
      }).catch(()=>{message.textContent="MeshMapper measurement cache is unavailable. The predicted terrain layer is still independent.";});
    }
    toggle.checked=new URLSearchParams(location.search).get("measured")==="1";
    toggle.addEventListener("change",update);
    if (toggle.checked) update();
  }
  return {validBounds,normalize,indexCells,visibleCells,timestampMs,describe,colour,canvasLayer,init};
});
