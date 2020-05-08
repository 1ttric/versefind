import React, {useEffect, useRef, useState} from 'react';
import './App.css';
import Button from "@material-ui/core/Button";
import Toolbar from "@material-ui/core/Toolbar";
import SearchIcon from '@material-ui/icons/Search';
import LinearProgress from "@material-ui/core/LinearProgress";
import TextField from "@material-ui/core/TextField";
import InputAdornment from "@material-ui/core/InputAdornment";
import Box from "@material-ui/core/Box";
import List from "@material-ui/core/List";
import {useCookies} from 'react-cookie';
import Typography from "@material-ui/core/Typography";
import ExpansionPanel from "@material-ui/core/ExpansionPanel";
import ExpansionPanelSummary from "@material-ui/core/ExpansionPanelSummary";
import ExpansionPanelDetails from "@material-ui/core/ExpansionPanelDetails";
import ExpandMoreIcon from '@material-ui/icons/ExpandMore';
import Pagination from '@material-ui/lab/Pagination';
import axios from 'axios';
import {createMuiTheme, ThemeProvider} from '@material-ui/core/styles';
import MusicNoteIcon from '@material-ui/icons/MusicNote';
import {CssBaseline} from "@material-ui/core";
import Slide from "@material-ui/core/Slide";
import KeyboardBackspaceIcon from '@material-ui/icons/KeyboardBackspace';
import Fade from "@material-ui/core/Fade";
import IconButton from "@material-ui/core/IconButton";


const LYRICS = [
    {
        artist: "Nirvana",
        title: "All Apologies",
        text: "I wish I was like you\nEasily amused\nFind my nest of salt\nEverything’s my fault"
    },
    {
        artist: "Nine Inch Nails",
        title: "Hurt",
        text: "And you could have it all\nMy empire of dirt\nI will let you down\nI will make you hurt"
    },
    {
        artist: "Joy Division",
        title: "Love Will Tear Us Apart",
        text: "Why is the bedroom so cold turned away on your side?\nIs my timing that flawed, our respect run so dry"
    },
    {
        artist: "Arcade Fire",
        title: "Sprawl II Mountains Beyond Mountains",
        text: "They heard me singing and they told me to stop\nQuit these pretentious things and just punch the clock"
    },
    {
        artist: "Beyonce",
        title: "Formation",
        text: "I like my baby hair, with baby hair and afros\nI like my negro nose with Jackson Five nostrils\nEarned all this money but they never take the country out me\nI got a hot sauce in my bag, swag"
    },
    {
        artist: "Laura Marling",
        title: "Ghosts",
        text: "Lover, please do not\nFall to your knees\nIt’s not like I believe in\nEverlasting love"
    },
    {
        artist: "LCD Soundsystem",
        title: "Losing My Edge",
        text: "I’m losing my edge\nTo all the kids in Tokyo and Berlin\nI’m losing my edge to the art-school Brooklynites in little jackets and borrowed nostalgia for the unremembered eighties"
    },
    {
        artist: "Leonard Cohen",
        title: "So Long, Marianne",
        text: "Well you know that I love to live with you\nbut you make me forget so very much\nI forget to pray for the angels\nAnd then the angels forget to pray for us"
    },
    {
        artist: "The Libertines",
        title: "Can't Stand Me Now",
        text: "An end fitting for the start\nYou twist and tore our love apart"
    },
    {
        artist: "Kate Bush",
        title: "Cloudbusting",
        text: "You’re like my yo-yo/ That glowed in the dark\nWhat made it special\nMade it dangerous\nSo I bury it\nAnd forget"
    },
    {
        artist: "Nick Cave",
        title: "Into My Arms",
        text: "I don’t believe in an interventionist God\nBut I know, darling, that you do\nBut if I did I would kneel down and ask Him\nNot to intervene when it came to you"
    },
    {
        artist: "Sister Of Mercy",
        title: "This Corrosion",
        text: "On days like this\nIn times like these\nI feel an animal deep inside\nHeel to haunch on bended knees"
    },
    {
        artist: "Sultans of Ping FC",
        title: "Where's Me Jumper",
        text: "It’s alright to say things can only get better\nYou haven’t lost your brand new sweater\nPure new wool, and perfect stitches\nNot the type of jumper that makes you itches"
    },
    {
        artist: "The Smiths",
        title: "There Is A Light That Never Goes Out",
        text: "Take me out tonight\nTake me anywhere, I don’t care\nI don’t care, I don’t care"
    },
    {
        artist: "Bruce Springsteen",
        title: "I'm On Fire",
        text: "At night I wake up with the sheets soaking wet\nAnd a freight train running through the\nMiddle of my head\nOnly you can cool my desire"
    }
];

function App() {
    const ws = useRef(null);
    const audio = useRef(new Audio(""));
    const [cookies, , removeCookie] = useCookies(["session"]);

    useEffect(() => {
        if (ws.current) return;
        ws.current = new WebSocket(window.location.origin.replace(/^http/, 'ws') + "/ws");
        ws.current.onopen = () => console.log("Websocket connected");
        ws.current.onerror = (evt) => {
            console.log("Websocket error", evt);
        };
        ws.current.onclose = (evt) => {
            // This custom websocket close code indicates our Spotify API token was not found or is otherwise invalid
            // In this case we will redirect back to the homepage
            if (evt.code === 4000) {
                removeCookie("session");
            }
        };
    });
    const theme = createMuiTheme({palette: {type: 'dark'},});
    return (
        <ThemeProvider theme={theme}>
            <CssBaseline/>
            {cookies.session ? <SearchScreen ws={ws} audio={audio}/> : <LoginScreen/>}
        </ThemeProvider>
    );
}

function LoginScreen() {
    const rotating = useRef(false);
    const rotatingContinuous = useRef(false);
    const lyricIndex = useRef(Math.floor(Math.random() * LYRICS.length));
    const [when, setWhen] = useState(true);

    const rotateText = () => {
        if (rotating.current) return;
        rotating.current = true;
        setWhen(false);
        setTimeout(() => {
            let newLyricIndex = lyricIndex.current;
            while (newLyricIndex === lyricIndex.current) {
                newLyricIndex = Math.floor(Math.random() * LYRICS.length)
            }
            lyricIndex.current = newLyricIndex;
            setWhen(true);
            rotating.current = false;
        }, 2000);
    };

    const continuousRotateText = () => {
        if (rotatingContinuous.current) return;
        setInterval(rotateText, 7000);
        rotatingContinuous.current = true;
    };

    const countNewLines = (text) => (text.match(/\n/g) || []).length;

    useEffect(continuousRotateText);

    let lyric = LYRICS[lyricIndex.current];
    let lyricTextPadded = lyric.text;
    // Pad lyrics vertically to greatest line count
    lyricTextPadded += "\n ".repeat(LYRICS.map(({text}) => countNewLines(text)).reduce((a, b) => Math.max(a, b)) - countNewLines(lyric.text) + 1);

    return (
        <Box style={{
            display: "flex",
            flexDirection: "column",
            justifyContent: "space-evenly",
            alignItems: "center",
            height: "100vh"
        }}>
            <Fade in={when} timeout={2000}>
                <Box mx={7} style={{
                    userSelect: "none",
                    flex: "5",
                    display: "flex",
                    flexDirection: "column",
                    justifyContent: "flex-end"
                }}>
                    <Box>
                        <Typography variant="h5" component="h2">
                            {lyric.title}
                        </Typography>
                    </Box>
                    <Typography color="textSecondary">
                        {lyric.artist}
                    </Typography>
                    <Box mt={1} ml={3} fontWeight="fontWeightLight" fontStyle="oblique">
                        <Typography variant="body2" style={{whiteSpace: "pre-wrap"}}>
                            {lyricTextPadded}
                        </Typography>
                    </Box>
                </Box>
            </Fade>
            <div style={{flex: "4"}}>
                <Box mt={3}>
                    <Button href="/auth">Search</Button>
                </Box>
            </div>
        </Box>
    );

}

function SearchScreen({ws, audio}) {
    const pageSize = 20;

    const [status, setStatus] = useState(null);
    const [query, setQuery] = useState("*");
    const [page, setPage] = useState(0);
    const [searchTimeout, setSearchTimeout] = useState();
    const [searchCount, setSearchCount] = useState(0);
    const [searchResult, setSearchResult] = useState([]);
    const [, , removeCookie] = useCookies(["session"]);

    // Wait until the websocket has been created and then set a listener
    useEffect(() => {
        let interval = setInterval(() => {
            if (!ws.current || ws.current.onmessage) return;
            ws.current.onmessage = (msg) => {
                let status = JSON.parse(msg.data);
                if (status.complete) {
                    // Now that we've received the last progress report, refresh the search screen
                    if (!searchResult.length) {
                        doSearch(query, 0).then()
                    }
                }
                setStatus(status);
            };
            clearInterval(interval)
        }, 100)
    });

    const doSearch = async (thisQuery, page) => {
        console.log(`Performing search with q=${thisQuery}, offset=${page * pageSize}, limit=${pageSize}`);
        setSearchResult([]);
        setQuery(thisQuery);
        let resp;
        try {
            resp = await axios.get("/search", {params: {q: thisQuery, offset: page * pageSize, limit: pageSize}});
        } catch (err) {
            console.log("Search failed - defaulting");
            setSearchResult(null);
            setSearchCount(0);
            return
        }
        console.log(`Search returned ${resp.data.total} results`);
        setSearchResult(resp.data);
        setSearchCount(resp.data.total);
    };


    let numPages = Math.ceil((searchCount || 0) / pageSize);
    return (
        <Box style={{display: "flex", flexDirection: "column", height: "100vh", alignItems: "stretch"}}>
            <Box my={2}>
                <Toolbar>
                    <TextField variant={"outlined"}
                               placeholder={"Search"}
                               defaultValue={"*"}
                               fullWidth={true}
                               onChange={async (evt) => {
                                   clearTimeout(searchTimeout);
                                   let query = evt.target.value;
                                   let timeout = setTimeout(async () => await doSearch(query, page), 300);
                                   setSearchTimeout(timeout);
                               }}
                               onKeyDown={(evt) => {
                                   clearTimeout(searchTimeout);
                                   let query = evt.target.value;
                                   evt.keyCode === 13 && doSearch(query, page)
                               }}
                               InputProps={{
                                   startAdornment: (
                                       <InputAdornment position="start">
                                           <SearchIcon/>
                                       </InputAdornment>
                                   ),
                               }}/>
                    <Box ml={2}>
                        <IconButton>
                            <KeyboardBackspaceIcon onClick={() => removeCookie("session")}/>
                        </IconButton>
                    </Box>
                </Toolbar>
            </Box>
            <List style={{flex: "1 1 0", overflowY: "auto"}}>
                {(searchResult?.results || []).map((track, idx) => <Track key={idx} trackData={track} audio={audio}/>)}
            </List>
            {numPages > 1 ? <Box my={2} style={{alignSelf: "center"}}>
                <Pagination size="small"
                            count={numPages}
                            showFirstButton
                            showLastButton
                            onChange={async (evt, page) => {
                                setPage(page - 1);
                                await doSearch(query, page - 1)
                            }}/></Box> : null}
            <Slide direction="up" in={status ? !status.complete : false} mountOnEnter unmountOnExit>
                <Box m={2} style={{display: "flex", alignItems: "center", textAlign: "center"}}>
                    <Box>{status?.text}<br/>{status?.n}/{status?.total}</Box>
                    <Box ml={1} flexGrow={1}>
                        <LinearProgress variant="determinate" value={status?.n / status?.total * 100}/>
                    </Box>
                </Box>
            </Slide>
        </Box>
    )
}

function Track({trackData, audio}) {
    let songTitle = trackData.spotify.name + " - " + trackData.spotify.artists.map(artist => artist.name).join(", ");
    let songLink = trackData.spotify.external_urls.spotify;
    let songImage = trackData.spotify.album.images[1]?.url;
    let previewLink = trackData.spotify.preview_url;
    let [imageLoaded, setImageLoaded] = useState(false);


    const playAudio = () => {
        try {
            if (audio.current.src !== previewLink) {
                audio.current.src = previewLink; // If the audio source is empty because of a fresh render or unload, populate (or repopulate) its src attribute
            }
            if (audio.current.src === "") {
                return
            }
            audio.current.play()
        } catch (err) {
            console.log("Error previewing track", err)
        }
    };
    const pauseAudio = () => audio.current.pause();
    const openSong = () => window.open(songLink, '_blank')?.focus();
    return (
        <Box mx={3} my={1} style={{display: "flex"}}>
            <Box onClick={openSong}
                 onMouseEnter={playAudio}
                 onMouseLeave={pauseAudio}
                 mr={1}
                 style={{flex: "0 0 auto", cursor: "pointer"}}>
                <div style={{width: "10vh"}}>
                    <div style={{
                        paddingBottom: "100%",
                        position: "relative",
                        display: imageLoaded ? "none" : null
                    }}>
                        <div style={{
                            position: "absolute",
                            inset: 0,
                            display: "flex",
                            alignItems: "center",
                            justifyContent: "center"
                        }}>
                            <MusicNoteIcon/>
                        </div>
                    </div>

                    <img src={songImage}
                         alt={"Album art"}
                         onLoad={() => setImageLoaded(true)}
                         style={{
                             objectFit: "cover",
                             width: "100%",
                             display: imageLoaded ? "block" : "none",
                             backgroundSize: "cover",
                             backgroundRepeat: "no-repeat",
                             backgroundPosition: "center",
                         }}/>
                </div>
            </Box>

            <ExpansionPanel elevation={0}
                            TransitionProps={{unmountOnExit: true}}
                            style={{flex: "1"}}>
                <ExpansionPanelSummary expandIcon={
                    trackData.lyrics ? (<ExpandMoreIcon/>) : null
                }
                                       disabled={!trackData.lyrics}>

                    <Box ml={1}>
                        <Typography>{songTitle}</Typography>
                    </Box>
                </ExpansionPanelSummary>
                <ExpansionPanelDetails>
                    <Box m={2}>
                        <Typography style={{whiteSpace: "break-spaces"}}>
                            {trackData.lyrics}
                        </Typography>
                    </Box>
                </ExpansionPanelDetails>
            </ExpansionPanel>
        </Box>
    );
}

export default App;
