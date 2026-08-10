"""The upstream half of every logical app the parity harness compares.

Each entry is a Streamlit script, executed headlessly through
``streamlit.testing.v1.AppTest.from_string``. The Go half of the same logical
app lives in ``../go/apps.go`` and must produce the same element tree.

Conventions shared with the Go side:

* Every widget a case needs to drive carries an explicit ``key``, except in the
  ``auto-keys`` app, which exists precisely to probe key generation.
* What the app *observed* (the value a widget call returned) is published into
  session state under an ``ss_`` prefix rather than rendered as text, so the
  two runners compare JSON numbers instead of two languages' float formatting.
"""

# --------------------------------------------------------------- widget snippets
#
# One line per widget, so the same source can be assembled into a
# single-widget app (granular scoring) and into the combined app.

WIDGET_SNIPPETS: dict[str, str] = {
    "button": 'st.session_state["ss_v"] = bool(st.button("Go", key="w_button"))',
    "checkbox": 'st.session_state["ss_v"] = st.checkbox("Agree", value=True, key="w_checkbox")',
    "radio": 'st.session_state["ss_v"] = st.radio("Pick", ["alpha", "beta", "gamma"], key="w_radio")',
    "selectbox": 'st.session_state["ss_v"] = st.selectbox("Choose", ["alpha", "beta", "gamma"], key="w_selectbox")',
    "multiselect": 'st.session_state["ss_v"] = sorted(st.multiselect("Many", ["alpha", "beta", "gamma"], key="w_multiselect"))',
    "slider": 'st.session_state["ss_v"] = st.slider("Amount", min_value=0.0, max_value=10.0, value=3.0, step=1.0, key="w_slider")',
    "text_input": 'st.session_state["ss_v"] = st.text_input("Name", value="ada", key="w_text_input")',
    "text_area": 'st.session_state["ss_v"] = st.text_area("Bio", value="hello", key="w_text_area")',
    "number_input": 'st.session_state["ss_v"] = st.number_input("Count", value=5.0, key="w_number_input")',
    "date_input": 'st.session_state["ss_v"] = st.date_input("Day", value=datetime.date(2020, 1, 2), key="w_date_input").isoformat()',
    "time_input": 'st.session_state["ss_v"] = st.time_input("Moment", value=datetime.time(9, 30), key="w_time_input").strftime("%H:%M")',
    "file_uploader": (
        '_f = st.file_uploader("Upload", key="w_file_uploader")\n'
        'st.session_state["ss_v"] = [] if _f is None else [_f.name]'
    ),
}

PREAMBLE = "import streamlit as st\nimport datetime\n\n"

APPS: dict[str, str] = {}

for _name, _src in WIDGET_SNIPPETS.items():
    APPS["wd-" + _name] = PREAMBLE + _src + "\n"

APPS["widgets-defaults"] = PREAMBLE + "\n".join(WIDGET_SNIPPETS.values()) + "\n"


def _add(name: str, body: str) -> None:
    APPS[name] = PREAMBLE + body.strip("\n") + "\n"


# --------------------------------------------------------------- text elements

_add(
    "text-elements",
    """
st.title("Parity")
st.header("Section")
st.subheader("Subsection")
st.markdown("**bold** and `code`")
st.caption("a caption")
st.latex("x^2 + y^2")
st.code("print(1)", language="python")
st.text("plain text")
st.divider()
st.success("all good")
st.info("for your information")
st.warning("be careful")
st.error("it broke")
""",
)

_add(
    "write-dispatch",
    """
st.write("a string")
st.write(3)
st.write(True)
""",
)

# --------------------------------------------------- widget presentation props

_add(
    "widgets-labelled",
    """
st.checkbox("Agree", value=True, key="w_checkbox", help="tick it", disabled=True)
st.slider(
    "Amount", min_value=0.0, max_value=10.0, value=3.0, step=1.0, format="%.1f",
    key="w_slider", help="slide it",
)
st.text_input(
    "Name", value="ada", key="w_text_input", max_chars=8, placeholder="who?",
    help="type it", disabled=True,
)
""",
)

# ------------------------------------------------------- clamping / validation

_add(
    "slider-bounds",
    """
st.session_state["ss_v"] = st.slider(
    "Amount", min_value=0.0, max_value=10.0, value=3.0, step=1.0, key="w_slider"
)
""",
)

_add(
    "number-bounds",
    """
st.session_state["ss_v"] = st.number_input(
    "Count", min_value=0.0, max_value=10.0, value=5.0, key="w_number_input"
)
""",
)

_add(
    "selectbox-unknown",
    """
st.session_state["ss_v"] = st.selectbox(
    "Choose", ["alpha", "beta", "gamma"], key="w_selectbox"
)
""",
)

_add(
    "multiselect-unknown",
    """
st.session_state["ss_v"] = sorted(
    st.multiselect("Many", ["alpha", "beta", "gamma"], key="w_multiselect")
)
""",
)

# ----------------------------------------------------------------------- layout

_add(
    "layout",
    """
left, right = st.columns(2)
left.text("left")
right.text("right")

first, second = st.tabs(["One", "Two"])
first.text("in one")
second.text("in two")

with st.expander("More", expanded=True):
    st.text("inside expander")

with st.container():
    st.text("inside container")

st.sidebar.text("in sidebar")
""",
)

_add(
    "layout-nested",
    """
cols = st.columns(2)
with cols[0]:
    with st.expander("Nested", expanded=False):
        st.text("deep")
cols[1].text("shallow")
""",
)

_add(
    "layout-widget-in-column",
    """
cols = st.columns(2)
st.session_state["ss_v"] = cols[0].text_input("Name", value="ada", key="w_text_input")
cols[1].text("right")
""",
)

# ------------------------------------------------------------- tabular and json

_add(
    "dataframe",
    """
st.dataframe([{"name": "ada", "qty": "1"}, {"name": "bob", "qty": "2"}])
""",
)

_add(
    "table",
    """
st.table([{"name": "ada", "qty": "1"}, {"name": "bob", "qty": "2"}])
""",
)

_add(
    "json-element",
    """
st.json({"b": 2, "a": [1, 2]})
""",
)

_add(
    "metric",
    """
st.metric("Revenue", "1.2M", "+4.1%")
st.metric("Latency", "120ms", "-8ms")
st.metric("Flat", "0", "0")
""",
)

# ----------------------------------------------------------------------- charts

_add("chart-line", 'st.line_chart([1.0, 3.0, 2.0])')
_add("chart-area", 'st.area_chart([1.0, 3.0, 2.0])')
_add("chart-bar", 'st.bar_chart([1.0, 3.0, 2.0])')
_add(
    "chart-scatter",
    'st.scatter_chart({"x": [1.0, 2.0, 3.0], "y": [4.0, 5.0, 6.0]}, x="x", y="y")',
)

# ------------------------------------------------------------------------ forms

_add(
    "form-atomic",
    """
with st.form("signup"):
    name = st.text_input("Name", value="", key="f_name")
    age = st.number_input("Age", value=18.0, key="f_age")
    submitted = st.form_submit_button("Register")
st.session_state["ss_name"] = name
st.session_state["ss_age"] = age
st.session_state["ss_submitted"] = bool(submitted)
""",
)

# ---------------------------------------------------------------- session state

_add(
    "session-state",
    """
st.session_state["ss_runs"] = st.session_state.get("ss_runs", 0) + 1
if "ss_clicks" not in st.session_state:
    st.session_state["ss_clicks"] = 0
if st.button("Bump", key="w_button"):
    st.session_state["ss_clicks"] += 1
""",
)

_add(
    "button-one-run",
    """
st.session_state["ss_hit"] = bool(st.button("Fire", key="w_button"))
st.session_state["ss_runs"] = st.session_state.get("ss_runs", 0) + 1
""",
)

_add(
    "widget-hidden-then-shown",
    """
show = st.checkbox("Show", value=True, key="w_flag")
if show:
    st.session_state["ss_v"] = st.text_input("Name", value="ada", key="w_text_input")
else:
    st.session_state["ss_v"] = None
""",
)

# ---------------------------------------------------------------------- caching

_add(
    "cache-hits",
    """
if "ss_computed" not in st.session_state:
    st.session_state["ss_computed"] = 0


@st.cache_data
def double(n):
    st.session_state["ss_computed"] += 1
    return n * 2


n = st.number_input("N", value=2.0, key="w_number_input")
st.session_state["ss_out"] = double(round(n, 3))
""",
)

_add(
    "cache-invalidate",
    """
if "ss_computed" not in st.session_state:
    st.session_state["ss_computed"] = 0


@st.cache_data
def constant():
    st.session_state["ss_computed"] += 1
    return st.session_state["ss_computed"]


if st.button("Clear", key="w_button"):
    constant.clear()
st.session_state["ss_out"] = constant()
""",
)

# --------------------------------------------------------- stop / control flow

_add(
    "stop",
    """
st.text("before")
if st.checkbox("Halt", value=False, key="w_checkbox"):
    st.stop()
st.text("after")
""",
)

_add(
    "stop-swallowed",
    """
st.text("before")
try:
    st.stop()
except BaseException:
    st.text("swallowed")
st.text("after")
""",
)

# ------------------------------------------------------------------- auto keys

_add(
    "auto-keys",
    """
if st.checkbox("Show extra", value=False, key="w_flag"):
    st.slider("Extra", min_value=0.0, max_value=10.0, value=0.0, step=1.0)
st.session_state["ss_target"] = st.slider(
    "Target", min_value=0.0, max_value=100.0, value=5.0, step=1.0
)
""",
)
