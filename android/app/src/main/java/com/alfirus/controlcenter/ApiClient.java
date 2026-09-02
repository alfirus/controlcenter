package com.alfirus.controlcenter;
import retrofit2.Retrofit; import retrofit2.converter.gson.GsonConverterFactory;
import retrofit2.http.GET; import retrofit2.http.Query; import java.util.List;
public class ApiClient {
    public static final String BASE = "http://10.0.2.2:8080/v1/";
    public interface Service { @GET("workspaces") retrofit2.Call<DataWrap<List<Workspace>>> workspaces(); }
    public static class DataWrap<T>{ public T data; }
    public static class Workspace{ public String id; public String name; }
    public static Service svc(){ return new Retrofit.Builder().baseUrl(BASE).addConverterFactory(GsonConverterFactory.create()).build().create(Service.class); }
}
